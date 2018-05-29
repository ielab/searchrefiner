package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/preprocess"
	"github.com/hscells/groove/stats"
	"gopkg.in/olivere/elastic.v5"
	"net/http"
	"github.com/hscells/transmute"
	"github.com/hscells/transmute/pipeline"
	"log"
)

func handleTree(c *gin.Context) {
	c.HTML(http.StatusOK, "tree.html", nil)
}

func (s server) handleQuery(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	t := make(map[string]pipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}

	ss, err := stats.NewElasticsearchStatisticsSource(stats.ElasticsearchAnalysedField("stemmed"))
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}
	repr, err := cq.Representation()
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}
	analysed := preprocess.SetAnalyseField(repr.(cqr.CommonQueryRepresentation), ss)()

	b, err := json.Marshal(analysed)
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}

	q, err := elasticPipeline.Execute(string(b))
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}

	// After that, we need to unmarshal it to get the underlying structure.
	var tmpQuery map[string]interface{}
	x, err := q.String()
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}
	err = json.Unmarshal(bytes.NewBufferString(x).Bytes(), &tmpQuery)
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}
	result := tmpQuery["query"].(map[string]interface{})

	b, err = json.Marshal(result)
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}
	elasticQuery := bytes.NewBuffer(b).String()

	client, err := elastic.NewClient(elastic.SetURL(s.Config.Elasticsearch))
	if err != nil {
		log.Fatalln(err)
	}

	// Scroll search.
	resp, err := client.Search("med_stem_sim2").
		Type("doc").
		Query(elastic.NewRawStringQuery(elasticQuery)).
		Do(context.Background())
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}

	sp, err := q.StringPretty()
	if err != nil {
		c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(500, err)
		return
	}

	sr := searchResponse{
		TotalHits:          resp.Hits.TotalHits,
		TookInMillis:       resp.TookInMillis,
		OriginalQuery:      rawQuery,
		ElasticsearchQuery: sp,
		Documents:          make([]document, len(resp.Hits.Hits)),
	}

	for i, hit := range resp.Hits.Hits {
		var doc map[string]interface{}
		err = json.Unmarshal(*hit.Source, &doc)
		if err != nil {
			c.HTML(500, "error.html", errorpage{Error: err.Error(), BackLink: "/"})
			c.AbortWithError(500, err)
			return
		}

		sr.Documents[i] = document{
			ID:    hit.Id,
			Title: doc["title"].(string),
			Text:  doc["text"].(string),
		}
	}

	previousQueries = append(previousQueries, rawQuery)
	c.HTML(http.StatusOK, "query.html", sr)
}

func (s server) handleIndex(c *gin.Context) {
	if !s.UserState.IsLoggedIn(s.UserState.Username(c.Request)) {
		c.Redirect(http.StatusTemporaryRedirect, "/account/login")
	}
	// reverse the list
	q := make([]string, len(previousQueries))
	j := 0
	for i := len(previousQueries) - 1; i >= 0; i-- {
		q[j] = previousQueries[i]
		j++
	}
	c.HTML(http.StatusOK, "index.html", q)
}

func handleTransform(c *gin.Context) {
	b := c.PostForm("query")
	q := ""
	if len(b) > 0 {
		cq, err := cqrPipeline.Execute(b)
		if err != nil {
			c.AbortWithError(500, err)
			return
		}

		fmt.Println(cq)
		s, err := cq.StringPretty()
		if err != nil {
			c.AbortWithError(500, err)
			return
		}
		q = s
	}

	c.HTML(http.StatusOK, "transform.html", struct{ Query string }{q})
}

func handleClear(c *gin.Context) {
	previousQueries = []string{}
	c.Redirect(http.StatusPermanentRedirect, "/")
	return
}
