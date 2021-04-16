package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/combinator"
	gpipeline "github.com/hscells/groove/pipeline"
	"github.com/hscells/transmute"
	tpipeline "github.com/hscells/transmute/pipeline"
	"github.com/ielab/searchrefiner"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type QueryVisPlugin struct {
}

type cachedItem struct {
	query string
	lang  string
	seeds []combinator.Document
}

var (
	tokenCache = cache.New(1*time.Hour, 1*time.Hour)
)

const pluginStorageName = "queryvis_consent"

func (QueryVisPlugin) Startup(server searchrefiner.Server) {
}

func handleTree(s searchrefiner.Server, c *gin.Context, relevant ...combinator.Document) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	p := make(map[string]tpipeline.TransmutePipeline)
	p["medline"] = transmute.Medline2Cqr
	p["pubmed"] = transmute.Pubmed2Cqr

	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}
	username := s.Perm.UserState().Username(c.Request)
	if len(username) == 0 {
		if token, ok := c.GetQuery("token"); ok {
			username = token
		}
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	repr, err := cq.Representation()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return

	}

	if len(relevant) == 0 {
		relevant = s.Settings[username].Relevant
	}

	var root combinator.LogicalTree
	root, err = combinator.NewShallowLogicalTree(gpipeline.NewQuery("searchrefiner", "0", repr.(cqr.CommonQueryRepresentation)), s.Entrez, relevant)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	t := buildTree(root.Root, relevant...)

	t.NumRel = len(relevant)
	switch r := root.Root.(type) {
	case combinator.Combinator:
		t.NumRelRet = int(r.R)
	case combinator.Atom:
		t.NumRelRet = int(r.R)
	}

	var numRet int64
	if len(t.Nodes) > 0 {
		numRet = int64(t.Nodes[0].Value)
	}

	err = s.Storage[pluginStorageName].CreateBucket("consent")
	if err != nil {
		panic(err)
	}

	if v, err := s.Storage[pluginStorageName].GetValue("consent", username); err == nil {
		if v != "n" {
			log.Infof(fmt.Sprintf("[username=%s][query=%s][lang=%s][pmids=%v][numrel=%d][numret=%d][numrelret=%d]", username, rawQuery, lang, relevant, t.NumRel, numRet, t.NumRelRet))
		}
	}

	s.Queries[username] = append(s.Queries[username], searchrefiner.Query{
		Time:        time.Now(),
		QueryString: rawQuery,
		Language:    lang,
		NumRet:      numRet,
		NumRelRet:   int64(t.NumRelRet),
	})

	c.JSON(200, t)
}

func (QueryVisPlugin) Serve(s searchrefiner.Server, c *gin.Context) {
	if _, ok := s.Storage[pluginStorageName]; !ok {
		var err error
		s.Storage[pluginStorageName], err = searchrefiner.OpenPluginStorage("queryvis_consent")
		if err != nil {
			panic(err)
		}
	}

	if c.Request.Method == "POST" && (c.Query("tree") == "y") {
		var item cachedItem
		if token, ok := c.GetQuery("token"); ok {
			if i, ok := tokenCache.Get(token); ok {
				item = i.(cachedItem)
			}
		}
		handleTree(s, c, item.seeds...)
		return
	}

	username := s.Perm.UserState().Username(c.Request)
	if len(username) == 0 {
		if token, ok := c.GetQuery("token"); ok {
			username = token
		}
	}
	if c.Request.Method == "POST" && (len(c.Query("consent")) > 0) {
		s.Storage[pluginStorageName].PutValue("consent", username, c.Query("consent"))
	}

	var consent bool
	{
		c, err := s.Storage[pluginStorageName].GetValue("consent", username)
		if err != nil {
			panic(err)
		}
		consent = c != "n"
	}

	rawQuery := ""
	lang := ""

	if token, ok := c.GetQuery("token"); ok {
		if i, ok := tokenCache.Get(token); !ok {
			content, err := s.ApiGetQuerySeedFromExchangeServer(token)
			if err != nil {
				c.String(http.StatusOK, "invalid token")
				panic(err)
				return
			}
			rawQuery = content.Data["query"]
			lang = content.Data["lang"]
			var rel []combinator.Document
			if _, ok := content.Data["seeds"]; ok {
				err = json.Unmarshal([]byte(content.Data["seeds"]), &rel)
				if err != nil {
					c.String(http.StatusOK, err.Error())
					panic(err)
					return
				}
			}
			tokenCache.Set(token, cachedItem{
				query: rawQuery,
				lang:  lang,
				seeds: rel,
			}, cache.DefaultExpiration)
		} else {
			item := i.(cachedItem)
			rawQuery = item.query
			lang = item.lang
		}
	} else {
		rawQuery = c.PostForm("query")
		lang = c.PostForm("lang")
	}

	c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/queryvis/index.html"), struct {
		searchrefiner.Query
		View    string
		Consent bool
	}{
		Query:   searchrefiner.Query{QueryString: rawQuery, Language: lang, Plugins: s.Plugins, PluginTitle: "QueryVis"},
		View:    c.Query("view"),
		Consent: consent,
	}))
	return
}

func (QueryVisPlugin) PermissionType() searchrefiner.PluginPermission {
	return searchrefiner.PluginUser
}

func (QueryVisPlugin) Details() searchrefiner.PluginDetails {
	return searchrefiner.PluginDetails{
		Title:             "QueryVis",
		Description:       "Visualise queries as a syntax tree overlaid with retrieval statistics and other understandability visualisations.",
		Author:            "ielab",
		Version:           "01.Dec.2020",
		ProjectURL:        "ielab.io/searchrefiner",
		AcceptsQueryPosts: true,
	}
}

var Queryvis = QueryVisPlugin{}
