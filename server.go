package main

import (
	"gopkg.in/olivere/elastic.v5"
	"log"
	"github.com/gin-gonic/gin"
	"net/http"
	"github.com/hscells/transmute/backend"
	"github.com/hscells/transmute/lexer"
	"github.com/hscells/transmute/parser"
	"github.com/hscells/transmute/pipeline"
	"github.com/hscells/groove/combinator"
	"os"
	"io"
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
	seen            = combinator.NewFileQueryCache("file_cache")
)

func main() {
	var err error
	client, err = elastic.NewClient(elastic.SetURL("http://sef-is-017660:8200"))
	if err != nil {
		log.Fatal(err)
	}

	lf, err := os.OpenFile("web/static/log", os.O_WRONLY|os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	lf.Truncate(0)
	mw := io.MultiWriter(lf, os.Stdout)
	log.SetOutput(mw)

	log.Println("Setting up routes...")
	router := gin.Default()

	router.LoadHTMLFiles(
		// Views.
		"web/query.html", "web/index.html", "web/transform.html", "web/tree.html",
		// Components.
		"components/sidebar.tmpl.html", "components/util.tmpl.html",
	)
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
	log.Fatal(http.ListenAndServe(":9999", router))
}
