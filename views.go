package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"gopkg.in/olivere/elastic.v5"
	"net/http"
	"github.com/hscells/transmute"
	"github.com/hscells/transmute/pipeline"
	"log"
	"strings"
	"github.com/hscells/groove/analysis"
	"github.com/hscells/cqr"
	"fmt"
)

func handleTree(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")
	c.HTML(http.StatusOK, "tree.html", citemedQuery{QueryString: rawQuery, Language: lang})
}

func (s server) handleResults(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	if len(rawQuery) == 0 {
		c.Redirect(http.StatusFound, "/")
		return
	}

	t := make(map[string]pipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
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

	b, err := json.Marshal(repr)
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

	var client *elastic.Client
	if strings.Contains(s.Config.Elasticsearch, "localhost") {
		client, err = elastic.NewClient(elastic.SetURL(s.Config.Elasticsearch), elastic.SetSniff(false))
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		client, err = elastic.NewClient(elastic.SetURL(s.Config.Elasticsearch))
		if err != nil {
			log.Fatalln(err)
		}
	}

	// Scroll search.
	resp, err := client.Scroll(s.Config.Index).
		Type("doc").
		Query(elastic.NewRawStringQuery(elasticQuery)).
		Size(10).
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

	// We send through the fist couple of hits before infinity scrolling on the results page.
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

		docs[i].PublicationTypes = make([]string, len(doc["publication_types"].([]interface{})))
		for j, pubType := range doc["publication_types"].([]interface{}) {
			docs[i].PublicationTypes[j] = pubType.(string)
		}

		docs[i].MeSHHeadings = make([]string, len(doc["mesh_headings"].([]interface{})))
		for j, heading := range doc["mesh_headings"].([]interface{}) {
			docs[i].MeSHHeadings[j] = heading.(string)
		}

		docs[i].Authors = make([]string, len(doc["authors"].([]interface{})))
		for j, author := range doc["authors"].([]interface{}) {
			a := author.(map[string]interface{})
			docs[i].Authors[j] = fmt.Sprintf("%v %v", a["last_name"], a["first_name"])
		}
	}

	terms := analysis.QueryTerms(repr.(cqr.CommonQueryRepresentation))

	for i, term := range terms {
		terms[i] = strings.ToLower(strings.Replace(term, "*", "", -1))
	}

	sr := searchResponse{
		TotalHits:          resp.Hits.TotalHits,
		TookInMillis:       resp.TookInMillis,
		OriginalQuery:      rawQuery,
		ElasticsearchQuery: sp,
		ScrollID:           resp.ScrollId,
		Documents:          docs,
		Language:           lang,
	}

	c.HTML(http.StatusOK, "results.html", sr)
}

func (s server) handleQuery(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	if len(rawQuery) == 0 {
		c.HTML(http.StatusOK, "query.html", searchResponse{Language: "medline"})
		return
	}

	t := make(map[string]pipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
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

	b, err := json.Marshal(repr)
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

	var client *elastic.Client
	if strings.Contains(s.Config.Elasticsearch, "localhost") {
		client, err = elastic.NewClient(elastic.SetURL(s.Config.Elasticsearch), elastic.SetSniff(false))
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		client, err = elastic.NewClient(elastic.SetURL(s.Config.Elasticsearch))
		if err != nil {
			log.Fatalln(err)
		}
	}

	// Scroll search.
	resp, err := client.Search(s.Config.Index).
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
		Language:           lang,
	}

	username := s.UserState.Username(c.Request)

	// Reverse the list of previous queries.
	rev := make([]citemedQuery, len(s.Queries[username]))
	j := 0
	for i := len(s.Queries[username]) - 1; i >= 0; i-- {
		rev[j] = s.Queries[username][i]
		j++
	}
	sr.PreviousQueries = rev

	s.Queries[username] = append(s.Queries[username], citemedQuery{QueryString: rawQuery, Language: lang, NumRet: sr.TotalHits})
	c.HTML(http.StatusOK, "query.html", sr)
}

func (s server) handleIndex(c *gin.Context) {
	if !s.UserState.IsLoggedIn(s.UserState.Username(c.Request)) {
		c.Redirect(http.StatusTemporaryRedirect, "/account/login")
	}
	username := s.UserState.Username(c.Request)
	// reverse the list
	q := make([]citemedQuery, len(s.Queries[username]))
	j := 0
	for i := len(s.Queries[username]) - 1; i >= 0; i-- {
		q[j] = s.Queries[username][i]
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

		s, err := cq.StringPretty()
		if err != nil {
			c.AbortWithError(500, err)
			return
		}
		q = s
	}

	c.HTML(http.StatusOK, "transform.html", struct{ Query string }{q})
}

func (s server) handleClear(c *gin.Context) {
	username := s.UserState.Username(c.Request)
	log.Println(username)
	s.Queries[username] = []citemedQuery{}
	c.Redirect(http.StatusFound, "/")
	return
}
