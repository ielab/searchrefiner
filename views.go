package searchrefiner

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"github.com/hscells/transmute"
	tpipeline "github.com/hscells/transmute/pipeline"
	"github.com/hscells/cqr"
	"time"
	gpipeline "github.com/hscells/groove/pipeline"
	"github.com/hscells/groove/analysis"
)

func HandleTree(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")
	c.HTML(http.StatusOK, "tree.html", Query{QueryString: rawQuery, Language: lang})
}

func (s Server) HandleResults(c *gin.Context) {
	start := time.Now()
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	if len(rawQuery) == 0 {
		c.Redirect(http.StatusFound, "/")
		return
	}

	t := make(map[string]tpipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	cqString, err := cq.String()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	pubmedQuery, err := transmute.Cqr2Pubmed.Execute(cqString)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	q, err := pubmedQuery.String()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	pmids, err := s.Entrez.Search(q, s.Entrez.SearchSize(10))
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	docs, err := s.Entrez.Fetch(pmids)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	repr, err := cq.Representation()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	size, err := s.Entrez.RetrievalSize(repr.(cqr.CommonQueryRepresentation))
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	sr := searchResponse{
		Start:            len(docs),
		TotalHits:        int64(size),
		TookInMillis:     float64(time.Since(start).Nanoseconds()) / 1e-6,
		OriginalQuery:    rawQuery,
		TransformedQuery: q,
		Documents:        docs,
		Language:         lang,
	}

	c.HTML(http.StatusOK, "results.html", sr)
}

func (s Server) HandleQuery(c *gin.Context) {
	start := time.Now()

	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	if len(rawQuery) == 0 {
		c.HTML(http.StatusOK, "query.html", searchResponse{Language: "medline"})
		return
	}

	t := make(map[string]tpipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	repr, err := cq.Representation()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	transformed, err := cq.StringPretty()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	size, err := s.Entrez.RetrievalSize(repr.(cqr.CommonQueryRepresentation))
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	sr := searchResponse{
		TotalHits:        int64(size),
		TookInMillis:     float64(time.Since(start).Nanoseconds()) / 1e-6,
		OriginalQuery:    rawQuery,
		TransformedQuery: transformed,
		Language:         lang,
	}

	gq := gpipeline.NewQuery("searchrefiner", "0", repr.(cqr.CommonQueryRepresentation))
	sr.BooleanClauses, err = analysis.BooleanClauses.Execute(gq, s.Entrez)
	sr.BooleanFields, _ = analysis.BooleanFields.Execute(gq, s.Entrez)
	sr.BooleanKeywords, _ = analysis.BooleanKeywords.Execute(gq, s.Entrez)
	sr.MeshKeywords, _ = analysis.MeshKeywordCount.Execute(gq, s.Entrez)
	sr.MeshExploded, _ = analysis.MeshExplodedCount.Execute(gq, s.Entrez)
	sr.MeshAvgDepth, _ = analysis.MeshAvgDepth.Execute(gq, s.Entrez)
	sr.MeshMaxDepth, _ = analysis.MeshMaxDepth.Execute(gq, s.Entrez)

	username := s.UserState.Username(c.Request)

	// Reverse the list of previous queries.
	rev := make([]Query, len(s.Queries[username]))
	j := 0
	for i := len(s.Queries[username]) - 1; i >= 0; i-- {
		rev[j] = s.Queries[username][i]
		j++
	}
	sr.PreviousQueries = rev

	s.Queries[username] = append(s.Queries[username], Query{QueryString: rawQuery, Language: lang, NumRet: sr.TotalHits})
	c.HTML(http.StatusOK, "query.html", sr)
}

func (s Server) HandleIndex(c *gin.Context) {
	if !s.UserState.IsLoggedIn(s.UserState.Username(c.Request)) {
		c.Redirect(http.StatusTemporaryRedirect, "/account/login")
	}
	username := s.UserState.Username(c.Request)
	// reverse the list
	q := make([]Query, len(s.Queries[username]))
	j := 0
	for i := len(s.Queries[username]) - 1; i >= 0; i-- {
		q[j] = s.Queries[username][i]
		j++
	}
	c.HTML(http.StatusOK, "index.html", q)
}

func (s Server) HandlePlugins(c *gin.Context) {
	c.HTML(http.StatusOK, "plugins.html", s.Plugins)
}

func HandleTransform(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	t := make(map[string]tpipeline.TransmutePipeline)
	t["pubmed"] = transmute.Pubmed2Cqr
	t["medline"] = transmute.Medline2Cqr

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	q, err := cq.StringPretty()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	c.HTML(http.StatusOK, "transform.html", struct {
		Query    string
		Language string
	}{Query: q, Language: lang})
}

func (s Server) HandleClear(c *gin.Context) {
	username := s.UserState.Username(c.Request)
	s.Queries[username] = []Query{}
	c.Redirect(http.StatusFound, "/")
	return
}
