package main

import (
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/combinator"
	gpipeline "github.com/hscells/groove/pipeline"
	"github.com/hscells/transmute"
	tpipeline "github.com/hscells/transmute/pipeline"
	"github.com/ielab/searchrefiner"
	log "github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type QueryVisPlugin struct {
}

func handleTree(s searchrefiner.Server, c *gin.Context) {
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

	log.Infof("recieved a query %s in format %s", rawQuery, lang)

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
	var root combinator.LogicalTree
	root, err = combinator.NewShallowLogicalTree(gpipeline.NewQuery("searchrefiner", "0", repr.(cqr.CommonQueryRepresentation)), s.Entrez, s.Settings[username].Relevant)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	t := buildTree(root.Root, searchrefiner.GetSettings(s, c).Relevant...)

	t.NumRel = len(s.Settings[username].Relevant)
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
	if c.Request.Method == "POST" && (c.Query("tree") == "y") {
		handleTree(s, c)
		return
	}

	rawQuery := ""
	lang := ""

	if c.Request.Method == "GET" && c.Query("token") != "" {
		content := s.ApiGetQuerySeedFromExchangeServer(c.Query("token"))
		rawQuery = content.Data.Query
		lang = "pubmed"
	} else {
		rawQuery = c.PostForm("query")
		lang = c.PostForm("lang")
	}

	c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/queryvis/index.html"), struct {
		searchrefiner.Query
		View string
	}{searchrefiner.Query{QueryString: rawQuery, Language: lang}, c.Query("view")}))
	return
}

func (QueryVisPlugin) PermissionType() searchrefiner.PluginPermission {
	return searchrefiner.PluginUser
}

func (QueryVisPlugin) Details() searchrefiner.PluginDetails {
	return searchrefiner.PluginDetails{
		Title:       "QueryVis",
		Description: "Visualise queries as a syntax tree overlaid with retrieval statistics and other understandability visualisations.",
		Author:      "ielab",
		Version:     "12.Feb.2019",
		ProjectURL:  "ielab.io/searchrefiner",
	}
}

var Queryvis = QueryVisPlugin{}
