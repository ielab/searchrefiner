package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/hscells/groove/stats"
	"github.com/ielab/searchrefiner"
	"github.com/xyproto/permissionbolt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"plugin"
	"strings"
)

func main() {
	f, err := os.Open("config.json")
	if err != nil {
		log.Fatalln(err)
	}
	var c searchrefiner.Config
	err = json.NewDecoder(f).Decode(&c)
	if err != nil {
		log.Fatalln(err)
	}

	fs, err := ioutil.ReadDir(searchrefiner.PluginStoragePath)
	if err != nil {
		log.Fatalln(err)
	}

	storage := make(map[string]*searchrefiner.PluginStorage)
	for _, f := range fs {
		ps, err := searchrefiner.OpenPluginStorage(f.Name())
		if err != nil {
			log.Fatalln(err)
		}
		storage[f.Name()] = ps
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
	perm.AddUserPath("/plugins")

	perm.AddPublicPath("/account")
	perm.AddPublicPath("/static")
	perm.AddPublicPath("/help")
	perm.AddPublicPath("/error")
	perm.AddPublicPath("/api/username")

	perm.AddAdminPath("/admin")

	ss, err := stats.NewEntrezStatisticsSource(
		stats.EntrezOptions(stats.SearchOptions{Size: 100000, RunName: "searchrefiner"}),
		stats.EntrezTool("searchrefiner"),
		stats.EntrezEmail(c.Entrez.Email),
		stats.EntrezAPIKey(c.Entrez.APIKey))
	if err != nil {
		log.Fatalln(err)
	}

	s := searchrefiner.Server{
		Perm:     perm,
		Config:   c,
		Queries:  make(map[string][]searchrefiner.Query),
		Settings: make(map[string]searchrefiner.Settings),
		Entrez:   ss,
		Storage:  storage,
	}

	permissionHandler := func(c *gin.Context) {
		if perm.Rejected(c.Writer, c.Request) {
			c.HTML(500, "error.html", searchrefiner.ErrorPage{Error: "unauthorised user", BackLink: "/"})
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
	g.Use(gzip.Gzip(gzip.BestCompression))

	g.LoadHTMLFiles(append([]string{
		// Views.
		"web/query.html", "web/index.html", "web/transform.html",
		"web/account_create.html", "web/account_login.html", "web/admin.html",
		"web/help.html", "web/error.html", "web/results.html", "web/settings.html", "web/plugins.html",
	}, searchrefiner.Components...)...)

	g.Static("/static/", "./web/static")

	// Handle plugins.
	files, err := ioutil.ReadDir("plugin")
	if err != nil {
		log.Fatalln(err)
	}
	for _, file := range files {
		if file.IsDir() {
			// Open the shared object file that will become the plugin.
			p := file.Name()
			plug, err := plugin.Open(path.Join("plugin", p, "plugin.so"))
			if err != nil {
				panic(err)
			}

			// Grab the exported type.
			sym, err := plug.Lookup(strings.Title(p))
			if err != nil {
				log.Fatalln(err)
			}

			// Ensure the type implements the plugin.
			var handle searchrefiner.Plugin
			var ok bool
			if handle, ok = sym.(searchrefiner.Plugin); !ok {
				log.Fatalln("could not cast", p, "to plugin")
			}

			// Configure the permissions for this plugin.
			p = path.Join("/plugin/", p)
			switch handle.PermissionType() {
			case searchrefiner.PluginAdmin:
				perm.AddAdminPath(p)
			case searchrefiner.PluginPublic:
				perm.AddPublicPath(p)
			case searchrefiner.PluginUser:
				perm.AddUserPath(p)
			default:
				perm.AddPublicPath(p)
			}

			s.Plugins = append(s.Plugins, searchrefiner.InternalPluginDetails{
				URL:           p,
				PluginDetails: handle.Details(),
			})

			fmt.Println(path.Join("plugin", file.Name(), "static"))
			g.Static(path.Join(p, "static"), path.Join("plugin", file.Name(), "static"))

			// Register the handler with gin.
			g.GET(p, func(c *gin.Context) {
				handle.Serve(s, c)
			})
			g.POST(p, func(c *gin.Context) {
				handle.Serve(s, c)
			})
		}
	}

	// Administration.
	g.GET("/admin", s.HandleAdmin)
	g.POST("/admin/api/confirm", s.ApiAdminConfirm)
	g.POST("/admin/api/storage", s.ApiAdminUpdateStorage)
	g.POST("/admin/api/storage/delete", s.ApiAdminDeleteStorage)
	g.POST("/admin/api/storage/csv", s.ApiAdminCSVStorage)

	// Authentication views.
	g.GET("/account/login", searchrefiner.HandleAccountLogin)
	g.GET("/account/create", searchrefiner.HandleAccountCreate)

	// Authentication API.
	g.POST("/account/api/login", s.ApiAccountLogin)
	g.POST("/account/api/create", s.ApiAccountCreate)
	g.GET("/account/api/logout", s.ApiAccountLogout)
	g.GET("/api/username", s.ApiAccountUsername)

	// Main query interface.
	g.GET("/", s.HandleIndex)
	g.GET("/clear", s.HandleClear)
	g.POST("/query", s.HandleQuery)
	g.GET("/query", s.HandleQuery)
	g.POST("/results", s.HandleResults)
	g.GET("/results", s.HandleResults)
	g.POST("/api/scroll", s.ApiScroll)

	// Editor interface.
	g.GET("/transform", searchrefiner.HandleTransform)
	g.POST("/transform", searchrefiner.HandleTransform)
	g.POST("/api/transform", searchrefiner.ApiTransform)
	g.POST("/api/cqr2query", searchrefiner.ApiCQR2Query)
	g.POST("/api/query2cqr", searchrefiner.ApiQuery2CQR)
	g.GET("/api/history", s.ApiHistoryGet)
	g.POST("/api/history", s.ApiHistoryAdd)
	g.DELETE("/api/history", s.ApiHistoryDelete)

	// Settings page.
	g.GET("/settings", s.HandleSettings)
	g.POST("/api/settings/relevant", s.ApiSettingsRelevantSet)

	// Plugins page.
	g.GET("/plugins", s.HandlePlugins)

	// Other utility pages.
	g.GET("/help", func(c *gin.Context) {
		c.HTML(http.StatusOK, "help.html", nil)
	})

	mw := io.MultiWriter(lf, os.Stdout)
	log.SetOutput(mw)

	// Set a global server configuration variable so plugins have access to it.
	searchrefiner.ServerConfiguration = s

	fmt.Print(`
                      _           ___ _             
  ___ ___ ___ ___ ___| |_ ___ ___|  _|_|___ ___ ___ 
 |_ -| -_| .'|  _|  _|   |  _| -_|  _| |   | -_|  _|
 |___|___|__,|_| |___|_|_|_| |___|_| |_|_|_|___|_|  

 Harry Scells 2019
 harry@scells.me
 https://ielab.io/searchrefiner

`)
	log.Fatalln(g.Run(c.Host))
}
