package main

import (
	"encoding/json"
	"net/http"
	"github.com/gin-gonic/gin"
	"fmt"
	"github.com/hscells/groove/stats"
	"github.com/hscells/groove/preprocess"
	"github.com/hscells/cqr"
	"bytes"
	"gopkg.in/olivere/elastic.v5"
	"log"
	"context"
)

func handleTree(c *gin.Context) {
	c.HTML(http.StatusOK, "tree.html", nil)
}

func handleQuery(c *gin.Context) {
	rawQuery := c.PostForm("query")

	cq, err := cqrPipeline.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	ss, err := stats.NewElasticsearchStatisticsSource(stats.ElasticsearchAnalysedField("stemmed"))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	repr, err := cq.Representation()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	analysed := preprocess.SetAnalyseField(repr.(cqr.CommonQueryRepresentation), ss)()

	b, err := json.Marshal(analysed)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	q, err := elasticPipeline.Execute(string(b))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	// After that, we need to unmarshal it to get the underlying structure.
	var tmpQuery map[string]interface{}
	s, err := q.String()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	err = json.Unmarshal(bytes.NewBufferString(s).Bytes(), &tmpQuery)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	result := tmpQuery["query"].(map[string]interface{})

	b, err = json.Marshal(result)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	elasticQuery := bytes.NewBuffer(b).String()

	// Scroll search.
	resp, err := client.Search("med_stem_sim2").
		Index("med_stem_sim2").
		Type("doc").
		Query(elastic.NewRawStringQuery(elasticQuery)).
		Do(context.Background())
	if err != nil {
		log.Println(elasticQuery)
		c.AbortWithError(500, err)
		return
	}

	sp, err := q.StringPretty()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	sr := SearchResponse{
		TotalHits:          resp.Hits.TotalHits,
		TookInMillis:       resp.TookInMillis,
		OriginalQuery:      rawQuery,
		ElasticsearchQuery: sp,
		Documents:          make([]Document, len(resp.Hits.Hits)),
	}

	for i, hit := range resp.Hits.Hits {
		var doc map[string]interface{}
		err = json.Unmarshal(*hit.Source, &doc)
		if err != nil {
			c.AbortWithError(500, err)
			return
		}

		sr.Documents[i] = Document{
			Id:    hit.Id,
			Title: doc["title"].(string),
			Text:  doc["text"].(string),
		}
	}

	previousQueries = append(previousQueries, rawQuery)
	c.HTML(http.StatusOK, "query.html", sr)
}

func handleIndex(c *gin.Context) {
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
