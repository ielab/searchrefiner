package main

import (
	"gopkg.in/olivere/elastic.v5"
	"log"
	"github.com/gin-gonic/gin"
	"net/http"
	"context"
	"bytes"
	"encoding/json"
	"github.com/hscells/transmute/backend"
	"github.com/hscells/transmute/lexer"
	"github.com/hscells/transmute/parser"
	"github.com/hscells/transmute/pipeline"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/preprocess"
	"github.com/hscells/groove/stats"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove"
	"strconv"
	"fmt"
)

type Document struct {
	Id    string
	Title string
	Text  string
}

type SearchResponse struct {
	TotalHits          int64
	TookInMillis       int64
	Documents          []Document
	OriginalQuery      string
	ElasticsearchQuery string
}

type Node struct {
	Id    int    `json:"id"`
	Value int    `json:"value"`
	Level int    `json:"level"`
	Label string `json:"label"`
	Shape string `json:"shape"`
}

type Edge struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Value int    `json:"value"`
	Label string `json:"label"`
}

type Tree struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

var (
	client      *elastic.Client
	cqrPipeline = pipeline.NewPipeline(
		parser.NewMedlineParser(),
		backend.NewCQRBackend(),
		pipeline.TransmutePipelineOptions{
			LexOptions: lexer.LexOptions{
				FormatParenthesis: false,
			},
			RequiresLexing: true,
		})
	elasticPipeline = pipeline.NewPipeline(
		parser.NewCQRParser(),
		backend.NewElasticsearchCompiler(),
		pipeline.TransmutePipelineOptions{
			LexOptions: lexer.LexOptions{
				FormatParenthesis: false,
			},
			RequiresLexing: false,
		})
	apiPipeline = pipeline.NewPipeline(
		parser.NewCQRParser(),
		backend.NewMedlineBackend(),
		pipeline.TransmutePipelineOptions{
			LexOptions: lexer.LexOptions{
				FormatParenthesis: false,
			},
			RequiresLexing: false,
		})
	previousQueries []string
	seen            = make(map[uint64]combinator.LogicalTreeNode)
)

func buildAdjTree(query cqr.CommonQueryRepresentation, id, parent, level int, ss *stats.ElasticsearchStatisticsSource) (nid int, t Tree) {
	var docs int
	if d, ok := seen[combinator.HashCQR(query)]; ok {
		docs = len(d.Documents())
	} else {
		d, err := ss.Execute(groove.NewPipelineQuery("adj", 1, query), ss.SearchOptions())
		if err != nil {
			log.Println("something bad happened")
			log.Fatalln(err)
			panic(err)
		}
		combDocs := make(combinator.Documents, len(d))
		for i, doc := range d {
			id, err := strconv.ParseInt(doc.DocId, 10, 32)
			if err != nil {
				log.Println("something bad happened")
				log.Fatalln(err)
				panic(err)
			}
			combDocs[i] = combinator.Document(id)
		}
		switch q := query.(type) {
		case cqr.Keyword:
			seen[combinator.HashCQR(query)] = combinator.NewAtom(q, combDocs)
		}
		docs = len(d)
	}
	switch q := query.(type) {
	case cqr.Keyword:
		t.Nodes = append(t.Nodes, Node{id, docs, level, q.StringPretty(), "box"})
		t.Edges = append(t.Edges, Edge{parent, id, docs, strconv.Itoa(docs)})
		id++
	case cqr.BooleanQuery:
		t.Nodes = append(t.Nodes, Node{id, docs, level, q.StringPretty(), "circle"})
		if parent > 0 {
			t.Edges = append(t.Edges, Edge{parent, id, docs, strconv.Itoa(docs)})
		}
		this := id
		id++
		for _, child := range q.Children {
			var nt Tree
			id, nt = buildAdjTree(child, id, this, level+1, ss)
			t.Nodes = append(t.Nodes, nt.Nodes...)
			t.Edges = append(t.Edges, nt.Edges...)
		}
	}
	nid += id
	return
}

func buildTreeRec(node combinator.LogicalTreeNode, id, parent, level int, ss *stats.ElasticsearchStatisticsSource) (nid int, t Tree) {
	if node == nil {
		fmt.Println("top", node, id, parent, level)
		return
	}
	fmt.Println("good", node, id, parent, level)
	docs := node.Documents()
	switch n := node.(type) {
	case combinator.Combinator:
		t.Nodes = append(t.Nodes, Node{id, len(docs), level, n.String(), "circle"})
		if parent > 0 {
			t.Edges = append(t.Edges, Edge{parent, id, len(docs), strconv.Itoa(len(docs))})
		}
		this := id
		id++
		for _, child := range n.Clauses {
			if child == nil {
				fmt.Println("child", node, child, id, parent, level)
				continue
			}
			var nt Tree
			id, nt = buildTreeRec(child, id, this, level+1, ss)
			t.Nodes = append(t.Nodes, nt.Nodes...)
			t.Edges = append(t.Edges, nt.Edges...)
		}
	case combinator.Atom:
		t.Nodes = append(t.Nodes, Node{id, len(docs), level, n.String(), "box"})
		t.Edges = append(t.Edges, Edge{parent, id, len(docs), strconv.Itoa(len(docs))})
		id++
	case combinator.AdjAtom:
		id, t = buildAdjTree(n.Query(), id, parent, level, ss)
	}
	nid += id
	return
}

func buildTree(node combinator.LogicalTreeNode, ss *stats.ElasticsearchStatisticsSource) (t Tree) {
	_, t = buildTreeRec(node, 1, 0, 0, ss)
	return
}

func apiTree(c *gin.Context) {
	rawQuery := c.PostForm("query")

	log.Println(rawQuery)

	cq, err := cqrPipeline.Execute(rawQuery)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	log.Println(cq)

	ss := stats.NewElasticsearchStatisticsSource(stats.ElasticsearchAnalysedField("stemmed"),
		stats.ElasticsearchScroll(true),
		stats.ElasticsearchIndex("med_stem_sim2"),
		stats.ElasticsearchDocumentType("doc"),
		stats.ElasticsearchHosts("http://sef-is-017660:8200"),
		stats.ElasticsearchField("text"),
		stats.ElasticsearchSearchOptions(stats.SearchOptions{Size: 3000, RunName: "citemed"}),
	)
	repr, err := cq.Representation()
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	analysed := preprocess.SetAnalyseField(repr.(cqr.CommonQueryRepresentation), ss)()

	var root combinator.LogicalTree
	root, seen, err = combinator.NewLogicalTree(groove.NewPipelineQuery("citemed", 0, analysed.(cqr.CommonQueryRepresentation)), ss, seen)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	t := buildTree(root.Root, ss)

	c.JSON(200, t)
}

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

	ss := stats.NewElasticsearchStatisticsSource(stats.ElasticsearchAnalysedField("stemmed"))
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

func handleClear(c *gin.Context) {
	previousQueries = []string{}
	c.Status(http.StatusOK)
}

func main() {
	var err error
	client, err = elastic.NewClient(elastic.SetURL("http://sef-is-017660:8200"))
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Setting up routes...")
	router := gin.Default()

	router.LoadHTMLFiles("web/query.html", "web/index.html", "web/transform.html", "web/tree.html")
	router.Static("/static/", "./web/static")

	// Main query interface.
	router.GET("/", handleIndex)
	router.GET("/clear", handleClear)
	router.POST("/query", handleQuery)

	// Editor interface.
	router.GET("/transform", handleTransform)
	router.POST("/transform", handleTransform)
	router.POST("/api/transform", apiTransform)
	router.POST("/api/cqr2medline", apiTransform)
	router.POST("/api/medline2cqr", apiTransformMedline2CQR)

	// Visualisation interface.
	router.GET("/tree", handleTree)
	router.POST("/api/tree", apiTree)

	log.Println("let's go!")
	log.Fatal(http.ListenAndServe(":8080", router))
}
