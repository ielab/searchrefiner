package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/transmute/backend"
	"github.com/hscells/transmute/lexer"
	"github.com/hscells/transmute/parser"
	"github.com/hscells/transmute/pipeline"
	"github.com/xyproto/permissionbolt"
	"github.com/xyproto/pinterface"
	"log"
	"net/http"
	"os"
	"strings"
	"io"
)

var (
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
	seen = combinator.NewFileQueryCache("file_cache")
)

type config struct {
	NoAuthentication string
	AdminEmail       string
	Admins           []string
	Elasticsearch    string
	Index            string
}

type citemedQuery struct {
	QueryString     string
	Language        string
	NumRet          int64
	PreviousQueries []citemedQuery
	Relevant        []string
}

type server struct {
	UserState pinterface.IUserState
	Perm      pinterface.IPermissions
	Queries   map[string][]citemedQuery
	Settings  map[string]settings
	Config    config
}

//noinspection SpellCheckingInspection
type errorpage struct {
	Error    string
	BackLink string
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

	lf, err := os.OpenFile("web/static/log", os.O_WRONLY|os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	lf.Truncate(0)

	dbPath := "citemed.db"

	g := gin.Default()

	perm, err := permissionbolt.NewWithConf(dbPath)
	if err != nil {
		log.Fatalln(err)
	}

	perm.Clear()
	perm.AddUserPath("/tree")
	perm.AddUserPath("/query")
	perm.AddUserPath("/transform")
	perm.AddUserPath("/settings")
	perm.AddUserPath("/api")

	perm.AddPublicPath("/account")
	perm.AddPublicPath("/static")
	perm.AddPublicPath("/help")
	perm.AddPublicPath("/error")

	perm.AddAdminPath("/admin")

	s := server{
		UserState: perm.UserState(),
		Perm:      perm,
		Config:    c,
		Queries:   make(map[string][]citemedQuery),
		Settings:  make(map[string]settings),
	}

	permissionHandler := func(c *gin.Context) {
		if perm.Rejected(c.Writer, c.Request) {
			c.HTML(500, "error.html", errorpage{Error: "unauthorised user", BackLink: "/"})
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

	g.Use(permissionHandler)

	g.LoadHTMLFiles(
		// Views.
		"web/query.html", "web/index.html", "web/transform.html", "web/tree.html",
		"web/account_create.html", "web/account_login.html", "web/admin.html",
		"web/help.html", "web/error.html", "web/results.html", "web/settings.html",
		// Components.
		"components/sidebar.tmpl.html", "components/util.tmpl.html",
		"components/login.template.html",
	)
	g.Static("/static/", "./web/static")

	// Administration.
	g.GET("/admin", s.handleAdmin)
	g.POST("/admin/api/confirm", s.apiAdminConfirm)

	// Authentication views.
	g.GET("/account/login", handleAccountLogin)
	g.GET("/account/create", handleAccountCreate)

	// Authentication API.
	g.POST("/account/api/login", s.apiAccountLogin)
	g.POST("/account/api/create", s.apiAccountCreate)
	g.GET("/account/api/logout", s.apiAccountLogout)

	// Main query interface.
	g.GET("/", s.handleIndex)
	g.GET("/clear", s.handleClear)
	g.POST("/query", s.handleQuery)
	g.GET("/query", s.handleQuery)
	g.POST("/results", s.handleResults)
	g.GET("/results", s.handleResults)
	g.POST("/api/scroll", s.apiScroll)

	// Editor interface.
	g.GET("/transform", handleTransform)
	g.POST("/transform", handleTransform)
	g.POST("/api/transform", apiTransform)
	g.POST("/api/cqr2query", apiCQR2Query)
	g.POST("/api/query2cqr", apiQuery2CQR)

	// Visualisation interface.
	g.GET("/tree", handleTree)
	g.POST("/tree", handleTree)
	g.POST("/api/tree", s.apiTree)

	g.GET("/settings", s.handleSettings)
	g.POST("/api/settings/relevant", s.apiSettingsRelevantSet)

	// Other utility pages.
	g.GET("/help", func(c *gin.Context) {
		c.HTML(http.StatusOK, "help.html", nil)
	})

	mw := io.MultiWriter(lf, os.Stdout)
	log.SetOutput(mw)

	fmt.Print(`
 .d8888b.  d8b 888            888b     d888               888 
d88P  Y88b Y8P 888            8888b   d8888               888 
888    888     888            88888b.d88888               888 
888        888 888888 .d88b.  888Y88888P888  .d88b.   .d88888 
888        888 888   d8P  Y8b 888 Y888P 888 d8P  Y8b d88" 888 
888    888 888 888   88888888 888  Y8P  888 88888888 888  888 
Y88b  d88P 888 Y88b. Y8b.     888   "   888 Y8b.     Y88b 888 
 "Y8888P"  888  "Y888 "Y8888  888       888  "Y8888   "Y88888 

 Harry Scells 2018
 harrisen.scells@hdr.qut.edu.au
 https://ielab.io/citemed

`)
	g.Run("0.0.0.0:4853")
}
