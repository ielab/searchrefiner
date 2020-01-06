package searchrefiner

import (
	"bufio"
	"github.com/gin-gonic/gin"
	"github.com/hanglics/gocheck/pkg/checker"
	"github.com/hanglics/gocheck/pkg/loader"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/analysis"
	"github.com/hscells/guru"
	"github.com/hscells/transmute"
	"github.com/hscells/transmute/fields"
	tpipeline "github.com/hscells/transmute/pipeline"
	log "github.com/sirupsen/logrus"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type searchResponse struct {
	Start            int
	TotalHits        int64
	TookInMillis     float64
	OriginalQuery    string
	TransformedQuery string
	PreviousQueries  []Query
	Documents        []guru.MedlineDocument
	Language         string
	BooleanClauses   float64
	BooleanKeywords  float64
	BooleanFields    float64
	MeshKeywords     float64
	MeshExploded     float64
	MeshAvgDepth     float64
	MeshMaxDepth     float64
}

func (s Server) ApiScroll(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	if len(rawQuery) == 0 {
		c.Redirect(http.StatusFound, "/")
		return
	}

	t := make(map[string]tpipeline.TransmutePipeline)
	t["medline"] = transmute.Medline2Cqr
	t["pubmed"] = transmute.Pubmed2Cqr

	startString := c.PostForm("start")
	scroll, err := strconv.ParseInt(startString, 10, 64)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	type scrollResponse struct {
		Documents []guru.MedlineDocument
		Start     int
		Total     float64
		Finished  bool
	}

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	cqString, err := cq.String()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	pubmedQuery, err := transmute.Cqr2Pubmed.Execute(cqString)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	q, err := pubmedQuery.String()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	pmids, err := s.Entrez.Search(q, s.Entrez.SearchStart(int(scroll)), s.Entrez.SearchSize(10))
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	docs, err := s.Entrez.Fetch(pmids)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	repr, err := cq.Representation()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	total, err := s.Entrez.RetrievalSize(repr.(cqr.CommonQueryRepresentation))
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	finished := false
	if len(docs) == 0 || (startString == "0" && len(docs) == int(total)) {
		finished = true
	}

	log.Infof("[scroll]  %s:%s:%s:%f", lang, rawQuery, startString, total)

	c.JSON(http.StatusOK, scrollResponse{Documents: docs, Start: len(docs), Finished: finished, Total: total})
}

func HandleQueryValidation(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")
	absPathFields, _ := filepath.Abs("../searchrefiner/dictionary/fields.txt")
	fieldsDictionary := loader.LoadDictionary(absPathFields)
	absPath, _ := filepath.Abs("../searchrefiner/dictionary/words.txt")
	keywordDictionary := loader.LoadDictionary(absPath)

	lang = strings.ToLower(lang)

	var fieldsError []string

	if strings.ToLower(lang) == "medline" {
		scanner := bufio.NewScanner(strings.NewReader(rawQuery))
		var extractedFields []string
		for scanner.Scan() {
			temp := scanner.Text()
			line := temp[3:]
			reg := regexp.MustCompile(`\.([^.]+)\.`)
			rawFields := reg.FindAllStringSubmatch(line, -1)
			if len(rawFields) > 0 {
				extractedFields = append(extractedFields, rawFields[0][1])
			}
		}
		if len(extractedFields) > 0 {
			for _, i := range extractedFields {
				flag := checker.CheckWord(fieldsDictionary, strings.ToLower(i), 0)
				if !flag {
					fieldsError = append(fieldsError, i)
				}
			}
		}
	} else if strings.ToLower(lang) == "pubmed" {
		reg := regexp.MustCompile(`\[([^]]+)\]`)
		rawFields := reg.FindAllStringSubmatch(rawQuery, -1)
		if len(rawFields) > 0 {
			for _, i := range rawFields {
				flag := checker.CheckWord(fieldsDictionary, strings.ToLower(i[1]), 0)
				if !flag {
					fieldsError = append(fieldsError, i[1])
				}
			}
		}
	}
	p := make(map[string]tpipeline.TransmutePipeline)
	p["medline"] = transmute.Medline2Cqr
	p["pubmed"] = transmute.Pubmed2Cqr
	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}
	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	repr, err := cq.Representation()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
	}
	commonRepr := repr.(cqr.CommonQueryRepresentation)
	keywords := analysis.QueryKeywords(commonRepr)
	var spellErrors []string
	resp := make(map[string][]string)
	for i := 0; i < len(keywords); i++ {
		keyword := keywords[i].QueryString
		keyword = strings.ToLower(keyword)
		flag := checker.CheckWord(keywordDictionary, keyword, 0)
		if !flag {
			spellErrors = append(spellErrors, keyword)
		}
	}
	resp["keyword"] = spellErrors
	resp["fields"] = fieldsError
	c.JSON(http.StatusOK, resp)
}

func ApiTransform(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	t := make(map[string]tpipeline.TransmutePipeline)
	t["pubmed"] = transmute.Cqr2Pubmed
	t["medline"] = transmute.Cqr2Medline

	compiler := t["medline"]
	if v, ok := t[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	q, err := cq.StringPretty()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Data(http.StatusOK, "text/plain", []byte(q))
}

func ApiCQR2Query(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")

	log.Infof("[cqr2query] %s:%s", lang, rawQuery)

	p := make(map[string]tpipeline.TransmutePipeline)
	p["medline"] = transmute.Cqr2Medline
	p["pubmed"] = transmute.Cqr2Pubmed

	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	s, err := cq.StringPretty()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Data(http.StatusOK, "application/json", []byte(s))
}

func ApiQuery2CQR(c *gin.Context) {
	rawQuery := c.PostForm("query")
	lang := c.PostForm("lang")
	field := c.PostForm("field")

	p := make(map[string]tpipeline.TransmutePipeline)
	p["medline"] = transmute.Medline2Cqr
	p["pubmed"] = transmute.Pubmed2Cqr

	compiler := p["medline"]
	if v, ok := p[lang]; ok {
		compiler = v
	} else {
		lang = "medline"
	}

	log.Infof("[query2cqr] %s:%s:%s", field, lang, rawQuery)

	// Use the field parameter to change the default field mapping.
	if len(field) > 0 {
		compiler.Parser.FieldMapping["default"] = []string{field}
	}

	cq, err := compiler.Execute(rawQuery)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	s, err := cq.StringPretty()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Data(http.StatusOK, "application/json", []byte(s))
}

func (s Server) ApiHistoryGet(c *gin.Context) {
	if !s.Perm.UserState().IsLoggedIn(s.Perm.UserState().Username(c.Request)) {
		c.Status(http.StatusForbidden)
		return
	}
	username := s.Perm.UserState().Username(c.Request)
	// reverse the list
	q := make([]Query, len(s.Queries[username]))
	j := 0
	for i := len(s.Queries[username]) - 1; i >= 0; i-- {
		q[j] = s.Queries[username][i]
		j++
	}

	c.JSON(http.StatusOK, q)
	return
}

func (s Server) ApiHistoryAdd(c *gin.Context) {
	if !s.Perm.UserState().IsLoggedIn(s.Perm.UserState().Username(c.Request)) {
		c.Status(http.StatusForbidden)
		return
	}
	username := s.Perm.UserState().Username(c.Request)
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

	log.Infof("[addhistory] %s:%s:%s", username, rawQuery, lang)
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

	date, ok := c.GetPostForm("date")
	if ok {
		repr = cqr.NewBooleanQuery(cqr.AND, []cqr.CommonQueryRepresentation{
			repr.(cqr.CommonQueryRepresentation),
			cqr.NewKeyword(date, fields.PublicationDate),
		})
	}

	size, err := s.Entrez.RetrievalSize(repr.(cqr.CommonQueryRepresentation))
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	s.Queries[username] = append(s.Queries[username], Query{
		Time:        time.Now(),
		QueryString: rawQuery,
		Language:    lang,
		NumRet:      int64(size),
	})

	c.Status(http.StatusOK)
	return
}

func (s Server) ApiHistoryDelete(c *gin.Context) {
	if !s.Perm.UserState().IsLoggedIn(s.Perm.UserState().Username(c.Request)) {
		c.Status(http.StatusForbidden)
		return
	}

	username := s.Perm.UserState().Username(c.Request)
	delete(s.Queries, username)
	log.Infof("[deletehistory] %s", username)
	c.Status(http.StatusOK)
	return
}
