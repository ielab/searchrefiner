package searchrefiner

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hscells/guru"
	"github.com/hscells/transmute"
	tpipeline "github.com/hscells/transmute/pipeline"
	"net/http"
	"strconv"
)

type searchResponse struct {
	Start            int
	TotalHits        int64
	TookInMillis     float64
	OriginalQuery    string
	TransformedQuery string
	PreviousQueries  []Query
	Documents        []guru.MedlineDocument
	Language         string
	BooleanClauses   float64
	BooleanKeywords  float64
	BooleanFields    float64
	MeshKeywords     float64
	MeshExploded     float64
	MeshAvgDepth     float64
	MeshMaxDepth     float64
}

func (s Server) ApiScroll(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	if len(rawQuery) == 0 {
		c.Redirect(http.StatusFound, "/")
		return
	}

	t := make(map[string]tpipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	startString := c.PostForm("start")
	scroll, err := strconv.ParseInt(startString, 10, 64)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	type scrollResponse struct {
		Documents []guru.MedlineDocument
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
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	cqString, err := cq.String()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	pubmedQuery, err := transmute.Cqr2Pubmed.Execute(cqString)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	q, err := pubmedQuery.String()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	pmids, err := s.Entrez.Search(q, s.Entrez.SearchStart(int(scroll)), s.Entrez.SearchSize(10))
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	docs, err := s.Entrez.Fetch(pmids)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	finished := false
	if len(docs) == 0 {
		finished = true
	}

	c.JSON(http.StatusOK, scrollResponse{Documents: docs, Start: len(docs), Finished: finished})
}

func ApiTransform(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	t := make(map[string]tpipeline.TransmutePipeline)
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
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	q, err := cq.StringPretty()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Data(http.StatusOK, "text/plain", []byte(q))
}

func ApiCQR2Query(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	fmt.Println(rawQuery)

	p := make(map[string]tpipeline.TransmutePipeline)
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
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	fmt.Println(cq)

	s, err := cq.StringPretty()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	fmt.Println(s)

	c.Data(http.StatusOK, "application/json", []byte(s))
}

func ApiQuery2CQR(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")
	field := c.PostForm("field")

	p := make(map[string]tpipeline.TransmutePipeline)
	p["medline"] = transmute.Medline2Cqr
	p["pubmed"] = transmute.Pubmed2Cqr

	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	// Use the field parameter to change the default field mapping.
	if len(field) > 0 {
		compiler.Parser.FieldMapping["default"] = []string{field}
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	s, err := cq.StringPretty()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Data(http.StatusOK, "application/json", []byte(s))
}

func (s Server) ApiHistory(c *gin.Context) {
	if !s.Perm.UserState().IsLoggedIn(s.Perm.UserState().Username(c.Request)) {
		c.Status(http.StatusForbidden)
		return
	}
	username := s.Perm.UserState().Username(c.Request)
	// reverse the list
	q := make([]Query, len(s.Queries[username]))
	j := 0
	for i := len(s.Queries[username]) - 1; i >= 0; i-- {
		q[j] = s.Queries[username][i]
		j++
	}

	c.JSON(http.StatusOK, q)
	return
}

func (s Server) ApiHistoryDelete(c *gin.Context) {
	if !s.Perm.UserState().IsLoggedIn(s.Perm.UserState().Username(c.Request)) {
		c.Status(http.StatusForbidden)
		return
	}
	username := s.Perm.UserState().Username(c.Request)

	delete(s.Queries, username)

	c.Status(http.StatusOK)
	return
}
