package main

import (
	"github.com/gin-gonic/gin"
	"log"
	"fmt"
	"github.com/hscells/groove/stats"
	"github.com/hscells/groove/preprocess"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove"
)

func (s server) apiTree(c *gin.Context) {
	rawQuery := c.PostForm("query")

	log.Println(rawQuery)

	cq, err := cqrPipeline.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	log.Println(cq)

	ss, err := stats.NewElasticsearchStatisticsSource(stats.ElasticsearchAnalysedField("stemmed"),
		stats.ElasticsearchScroll(true),
		stats.ElasticsearchIndex("med_stem_sim2"),
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

	analysed := preprocess.SetAnalyseField(repr.(cqr.CommonQueryRepresentation), ss)()

	var root combinator.LogicalTree
	root, _, err = combinator.NewLogicalTree(groove.NewPipelineQuery("citemed", "0", analysed.(cqr.CommonQueryRepresentation)), ss, seen)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	t := buildTree(root.Root, ss)

	c.JSON(200, t)
}

func apiTransform(c *gin.Context) {
	b, err := c.GetRawData()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	log.Println(string(b))

	q, err := apiPipeline.Execute(string(b))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	fmt.Println(q)

	s, err := q.StringPretty()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.Data(200, "text/plain", []byte(s))
}

func apiTransformMedline2CQR(c *gin.Context) {
	b, err := c.GetRawData()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	log.Println(string(b))

	q, err := cqrPipeline.Execute(string(b))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	fmt.Println(q)

	s, err := q.StringPretty()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.Data(200, "application/json", []byte(s))
}
