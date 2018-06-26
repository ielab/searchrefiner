package main

import (
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/groove"
	"github.com/hscells/groove/combinator"
	"net/http"
	"github.com/hscells/transmute/pipeline"
	"github.com/hscells/transmute"
	"strconv"
	"github.com/hscells/groove/stats"
)

type searchResponse struct {
	Start            int
	TotalHits        int64
	TookInMillis     float64
	OriginalQuery    string
	TransformedQuery string
	PreviousQueries  []citemedQuery
	Documents        []stats.EntrezDocument
	Language         string
}

type node struct {
	ID    int    `json:"id"`
	Value int    `json:"value"`
	Level int    `json:"level"`
	Label string `json:"label"`
	Shape string `json:"shape"`
}

type edge struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Value int    `json:"value"`
	Label string `json:"label"`
}

type tree struct {
	Nodes     []node `json:"nodes"`
	Edges     []edge `json:"edges"`
	relevant  map[combinator.Document]struct{}
	NumRelRet int
	NumRel    int
}

func (s server) apiTree(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	p := make(map[string]pipeline.TransmutePipeline)
	p["medline"] = transmute.Medline2Cqr
	p["pubmed"] = transmute.Pubmed2Cqr

	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	repr, err := cq.Representation()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	var root combinator.LogicalTree
	root, _, err = combinator.NewLogicalTree(groove.NewPipelineQuery("citemed", "0", repr.(cqr.CommonQueryRepresentation)), s.Entrez, seen)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	t := buildTree(root.Root, s.Entrez, getSettings(s, c).Relevant...)

	username := s.UserState.Username(c.Request)
	t.NumRel = len(s.Settings[username].Relevant)
	t.NumRelRet = len(t.relevant)

	c.JSON(200, t)
}

func (s server) apiScroll(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	if len(rawQuery) == 0 {
		c.Redirect(http.StatusFound, "/")
		return
	}

	t := make(map[string]pipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	startString := c.PostForm("start")
	scroll, err := strconv.ParseInt(startString, 10, 64)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	type scrollResponse struct {
		Documents []stats.EntrezDocument
		Start     int
		Finished  bool
	}

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	cqString, err := cq.String()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	pubmedQuery, err := transmute.Cqr2Pubmed.Execute(cqString)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	q, err := pubmedQuery.String()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	pmids, err := s.Entrez.Search(q, s.Entrez.SearchStart(int(scroll)), s.Entrez.SearchSize(10))
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	docs, err := s.Entrez.Fetch(pmids)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	finished := false
	if len(docs) == 0 {
		finished = true
	}

	c.JSON(http.StatusOK, scrollResponse{Documents: docs, Start: len(docs), Finished: finished})
}

func apiTransform(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	t := make(map[string]pipeline.TransmutePipeline)
	t["pubmed"] = transmute.Cqr2Pubmed
	t["medline"] = transmute.Cqr2Medline

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	q, err := cq.StringPretty()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.Data(200, "text/plain", []byte(q))
}

func apiCQR2Query(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	p := make(map[string]pipeline.TransmutePipeline)
	p["medline"] = transmute.Cqr2Medline
	p["pubmed"] = transmute.Cqr2Pubmed

	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	s, err := cq.StringPretty()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.Data(200, "application/json", []byte(s))
}

func apiQuery2CQR(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	p := make(map[string]pipeline.TransmutePipeline)
	p["medline"] = transmute.Medline2Cqr
	p["pubmed"] = transmute.Pubmed2Cqr

	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	s, err := cq.StringPretty()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.Data(200, "application/json", []byte(s))
}
