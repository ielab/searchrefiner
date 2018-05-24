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
	"github.com/xyproto/pinterface"
	"github.com/xyproto/permissionbolt"
	"encoding/json"
	"strings"
	"fmt"
	"time"
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

type config struct {
	NoAuthentication string
	AdminEmail       string
	Admins           []string
	Elasticsearch    string
}

type server struct {
	UserState pinterface.IUserState
	Perm      pinterface.IPermissions
	Config    config
}

func main() {
	f, err := os.Open("config.json")
	if err != nil {
		log.Fatalln(err)
	}
	var c config
	err = json.NewDecoder(f).Decode(&c)
	if err != nil {
		log.Fatalln(err)
	}

	client, err = elastic.NewClient(elastic.SetURL(c.Elasticsearch), elastic.SetSniff(false), elastic.SetHealthcheck(false), elastic.SetHealthcheckTimeout(time.Hour))
	if err != nil {
		log.Fatalln(err)
	}

	lf, err := os.OpenFile("web/static/log", os.O_WRONLY|os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	lf.Truncate(0)
	mw := io.MultiWriter(lf, os.Stdout)
	log.SetOutput(mw)

	dbPath := "citemed.db"

	router := gin.Default()

	perm, err := permissionbolt.NewWithConf(dbPath)
	if err != nil {
		log.Fatalln(err)
	}

	perm.Clear()
	perm.AddUserPath("/tree")
	perm.AddUserPath("/query")
	perm.AddUserPath("/transform")
	perm.AddUserPath("/api")
	perm.AddPublicPath("/account")
	perm.AddPublicPath("/static")
	perm.AddAdminPath("/admin")

	s := server{
		UserState: perm.UserState(),
		Perm:      perm,
		Config:    c,
	}

	permissionHandler := func(c *gin.Context) {
		if perm.Rejected(c.Writer, c.Request) {
			log.Println("unauthorised user")
			c.AbortWithStatus(http.StatusForbidden)
			return
		} else if len(perm.UserState().Username(c.Request)) > 0 && !perm.UserState().IsConfirmed(perm.UserState().Username(c.Request)) {
			if !strings.HasPrefix(c.Request.URL.Path, "/account") && !strings.HasPrefix(c.Request.URL.Path, "/static") {
				c.Data(http.StatusForbidden, "text/plain", []byte(fmt.Sprintf("Your account is waiting to be confirmed, please email %v if this takes longer than 24 hours.", s.Config.AdminEmail)))
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}
		c.Next()
	}

	router.Use(permissionHandler)

	router.LoadHTMLFiles(
		// Views.
		"web/query.html", "web/index.html", "web/transform.html", "web/tree.html",
		"web/account_create.html", "web/account_login.html", "web/admin.html",
		// Components.
		"components/sidebar.tmpl.html", "components/util.tmpl.html",
		"components/login.template.html",
	)
	router.Static("/static/", "./web/static")

	// Administration.
	router.GET("/admin", s.handleAdmin)
	router.POST("/admin/api/confirm", s.handleApiAdminConfirm)

	// Authentication views.
	router.GET("/account/login", handleAccountLogin)
	router.GET("/account/create", handleAccountCreate)

	// Authentication API.
	router.POST("/account/api/login", s.handleAuthAccountLogin)
	router.POST("/account/api/create", s.handleAuthAccountCreate)
	router.GET("/account/api/logout", s.handleAuthAccountLogout)

	// Main query interface.
	router.GET("/", s.handleIndex)
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
	router.POST("/api/tree", s.apiTree)

	log.Println("let's go!")
	router.Run("0.0.0.0:4853")
}
