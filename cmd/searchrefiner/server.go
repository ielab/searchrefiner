package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/hscells/groove/stats"
	"github.com/ielab/searchrefiner"
	log "github.com/sirupsen/logrus"
	"github.com/xyproto/permissionbolt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"plugin"
	"strings"
	"time"
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

	err = os.MkdirAll("logs", 0777)
	if err != nil {
		log.Fatalln(err)
	}

	t := time.Now().Unix()
	ginLf, err := os.OpenFile(fmt.Sprintf("logs/sr-gin-%d.log", t), os.O_WRONLY|os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	eveLf, err := os.OpenFile(fmt.Sprintf("logs/sr-eve-%d.log", t), os.O_WRONLY|os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalln(err)
	}

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.RFC3339,
	})

	log.SetOutput(io.MultiWriter(eveLf, os.Stdout))

	dbPath := "citemed.db"

	g := gin.Default()
	gin.DefaultWriter = io.MultiWriter(ginLf, os.Stdout)
	g.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// your custom format
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC3339),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	}))

	perm, err := permissionbolt.NewWithConf(dbPath)
	if err != nil {
		log.Fatalln(err)
	}

	perm.Clear()
	perm.AddUserPath("/query")
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

	g.Static("/static/", "./web/static")

	var pluginTemplates []string

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
			p = path.Join("./plugin/", p)
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

			pluginFiles, err := ioutil.ReadDir(p)
			if err != nil {
				panic(err)
			}
			for _, f := range pluginFiles {
				if !f.IsDir() {
					parts := strings.Split(f.Name(), ".")
					if len(parts) < 2 {
						continue
					}
					if parts[len(parts)-2] == "tmpl" && parts[len(parts)-1] == "html" {
						fmt.Println(path.Join(p, f.Name()))
						pluginTemplates = append(pluginTemplates, path.Join(p, f.Name()))
					}
				}
			}

			s.Plugins = append(s.Plugins, searchrefiner.InternalPluginDetails{
				URL:           p,
				PluginDetails: handle.Details(),
			})

			g.Static(path.Join(p, "static"), path.Join("plugin", file.Name(), "static"))

			if p == s.Config.Mode && s.Config.EnableAll == false {
				// Register the handler with gin.
				g.GET(p, func(c *gin.Context) {
					handle.Serve(s, c)
				})
				g.POST(p, func(c *gin.Context) {
					handle.Serve(s, c)
				})
				break
			} else if s.Config.EnableAll == true {
				// Register the handler with gin.
				g.GET(p, func(c *gin.Context) {
					handle.Serve(s, c)
				})
				g.POST(p, func(c *gin.Context) {
					handle.Serve(s, c)
				})
			}
		}
	}

	g.LoadHTMLFiles(append([]string{
		// Views.
		"web/query.html", "web/index.html", "web/transform.html",
		"web/account_create.html", "web/account_login.html", "web/admin.html",
		"web/help.html", "web/error.html", "web/results.html", "web/settings.html", "web/plugins.html",
	}, append(searchrefiner.Components)...)...)
	searchrefiner.PluginTemplates = pluginTemplates

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

	if c.EnableAll == true {
		// Main query interface.
		g.GET("/", s.HandleIndex)
		g.GET("/clear", s.HandleClear)
		g.POST("/query", s.HandleQuery)
		g.GET("/query", s.HandleQuery)
	} else {
		g.GET("/", func(ctx *gin.Context) {
			if !s.Perm.UserState().IsLoggedIn(s.Perm.UserState().Username(ctx.Request)) {
				ctx.Redirect(http.StatusTemporaryRedirect, "/account/login")
				return
			}
			ctx.Redirect(http.StatusFound, c.Mode)
		})
	}
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


	if s.Config.EnableAll == true {
		// Settings page.
		g.GET("/settings", s.HandleSettings)
		g.POST("/api/settings/relevant", s.ApiSettingsRelevantSet)

		// Plugins page.
		g.GET("/plugins", s.HandlePlugins)
	} else {
		// Settings page.
		g.GET("/settings", s.HandlePluginWithControl)
		g.POST("/api/settings/relevant", s.HandlePluginWithControl)

		// Plugins page.
		g.GET("/plugins", s.HandlePluginWithControl)
	}
  
	// Other utility pages.
	g.GET("/help", func(c *gin.Context) {
		c.HTML(http.StatusOK, "help.html", nil)
	})

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
