package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/combinator"
	gpipeline "github.com/hscells/groove/pipeline"
	"github.com/hscells/transmute"
	"github.com/hscells/transmute/fields"
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

	compiler.Options.FieldMapping = map[string][]string{
		"Affiliation":                     {fields.Affiliation},
		"All Fields":                      {fields.AllFields},
		"Author":                          {fields.Author},
		"Authors":                         {fields.Authors},
		"Author - Corporate":              {fields.AuthorCorporate},
		"Author - First":                  {fields.AuthorFirst},
		"Author - Full":                   {fields.AuthorFull},
		"Author - Identifier":             {fields.AuthorIdentifier},
		"Author - Last":                   {fields.AuthorLast},
		"Book":                            {fields.Book},
		"Date - Completion":               {fields.DateCompletion},
		"Conflict Of Interest Statements": {fields.ConflictOfInterestStatements},
		"Date - Create":                   {fields.DateCreate},
		"Date - Entrez":                   {fields.DateEntrez},
		"Date - MeSH":                     {fields.DateMeSH},
		"Date - Modification":             {fields.DateModification},
		"Date - Publication":              {fields.DatePublication},
		"EC/RN Number":                    {fields.ECRNNumber},
		"Editor":                          {fields.Editor},
		"Filter":                          {fields.Filter},
		"Grant Number":                    {fields.GrantNumber},
		"ISBN":                            {fields.ISBN},
		"Investigator":                    {fields.Investigator},
		"Investigator - Full":             {fields.InvestigatorFull},
		"Issue":                           {fields.Issue},
		"Journal":                         {fields.Journal},
		"Language":                        {fields.Language},
		"Location ID":                     {fields.LocationID},
		"MeSH Major Topic":                {fields.MeSHMajorTopic},
		"MeSH Subheading":                 {fields.MeSHSubheading},
		"MeSH Terms":                      {fields.MeSHTerms},
		"Other Term":                      {fields.OtherTerm},
		"Pagination":                      {fields.Pagination},
		"Pharmacological Action":          {fields.PharmacologicalAction},
		"Publication Type":                {fields.PublicationType},
		"Publisher":                       {fields.Publisher},
		"Secondary Source ID":             {fields.SecondarySourceID},
		"Subject Personal Name":           {fields.SubjectPersonalName},
		"Supplementary Concept":           {fields.SupplementaryConcept},
		"Floating MeshHeadings":           {fields.FloatingMeshHeadings},
		"Text Word":                       {fields.TextWord},
		"Title":                           {fields.Title},
		"Title/Abstract":                  {fields.TitleAbstract},
		"Transliterated Title":            {fields.TransliteratedTitle},
		"Volume":                          {fields.Volume},
		"MeSH Headings":                   {fields.MeshHeadings},
		"Major Focus MeSH Heading":        {fields.MajorFocusMeshHeading},
		"Publication Date":                {fields.PublicationDate},
		"Publication Status":              {fields.PublicationStatus},
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
