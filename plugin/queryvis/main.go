package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/combinator"
	gpipeline "github.com/hscells/groove/pipeline"
	"github.com/hscells/transmute"
	tpipeline "github.com/hscells/transmute/pipeline"
	"github.com/ielab/searchrefiner"
	"net/http"
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

	fmt.Println(rawQuery)

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	fmt.Println(cq)

	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	repr, err := cq.Representation()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	fmt.Println(repr)

	var root combinator.LogicalTree
	root, _, err = combinator.NewLogicalTree(gpipeline.NewQuery("searchrefiner", "0", repr.(cqr.CommonQueryRepresentation)), s.Entrez, searchrefiner.QueryCacher)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	t := buildTree(root.Root, s.Entrez, searchrefiner.GetSettings(s, c).Relevant...)

	username := s.Perm.UserState().Username(c.Request)
	t.NumRel = len(s.Settings[username].Relevant)
	t.NumRelRet = len(t.relevant)

	c.JSON(200, t)
}

func (QueryVisPlugin) Serve(s searchrefiner.Server, c *gin.Context) {
	if c.Request.Method == "POST" {
		handleTree(s, c)
		return
	}
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")
	c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/queryvis/index.html"), searchrefiner.Query{QueryString: rawQuery, Language: lang}))
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
