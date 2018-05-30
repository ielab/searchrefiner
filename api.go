package main

import (
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/groove"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/stats"
	"gopkg.in/olivere/elastic.v5"
	"strings"
	"context"
	"net/http"
	"encoding/json"
	"io"
	"github.com/hscells/transmute/pipeline"
	"github.com/hscells/transmute"
)

type document struct {
	ID    string
	Title string
	Text  string
}

type searchResponse struct {
	TotalHits          int64
	TookInMillis       int64
	OriginalQuery      string
	ElasticsearchQuery string
	PreviousQueries    []citemedQuery
	Documents          []document
	Language           string
	ScrollID           string
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
	Nodes []node `json:"nodes"`
	Edges []edge `json:"edges"`
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

	ss, err := stats.NewElasticsearchStatisticsSource(
		stats.ElasticsearchScroll(true),
		stats.ElasticsearchIndex(s.Config.Index),
		stats.ElasticsearchDocumentType("doc"),
		stats.ElasticsearchHosts(s.Config.Elasticsearch),
		stats.ElasticsearchField("text"),
		stats.ElasticsearchSearchOptions(stats.SearchOptions{Size: 800, RunName: "citemed"}),
	)
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
	root, _, err = combinator.NewLogicalTree(groove.NewPipelineQuery("citemed", "0", repr.(cqr.CommonQueryRepresentation)), ss, seen)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	t := buildTree(root.Root, ss)

	c.JSON(200, t)
}

func (s server) apiScroll(c *gin.Context) {
	scrollID := c.PostForm("scroll")

	type scrollResponse struct {
		Documents []document
		ScrollID  string
		Finished  bool
	}

	var client *elastic.Client
	var err error
	if strings.Contains(s.Config.Elasticsearch, "localhost") {
		client, err = elastic.NewClient(elastic.SetURL(s.Config.Elasticsearch), elastic.SetSniff(false))
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
	} else {
		client, err = elastic.NewClient(elastic.SetURL(s.Config.Elasticsearch))
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
	}

	finished := false
	resp, err := client.Scroll(s.Config.Index).ScrollId(scrollID).Size(1).Scroll("1h").Do(context.Background())
	if err == io.EOF {
		finished = true
		c.JSON(http.StatusOK, scrollResponse{Finished: finished})
		return
	} else if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	docs := make([]document, len(resp.Hits.Hits))

	for i, hit := range resp.Hits.Hits {
		var doc map[string]interface{}
		err = json.Unmarshal(*hit.Source, &doc)
		if err != nil {
			c.AbortWithError(500, err)
			return
		}

		docs[i] = document{
			ID:    hit.Id,
			Title: doc["title"].(string),
			Text:  doc["text"].(string),
		}
	}

	c.JSON(http.StatusOK, scrollResponse{Documents: docs, ScrollID: resp.ScrollId, Finished: finished})
}

func apiTransform(c *gin.Context) {
	b, err := c.GetRawData()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	q, err := apiPipeline.Execute(string(b))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	s, err := q.StringPretty()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.Data(200, "text/plain", []byte(s))
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
