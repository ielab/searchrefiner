package searchrefiner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bbalet/stopwords"
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/guru"
	"github.com/hscells/transmute"
	"github.com/hscells/transmute/fields"
	tpipeline "github.com/hscells/transmute/pipeline"
	"github.com/olivere/elastic"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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

type singleSuggestion struct {
	Score	float64 `json:"score"`
	Term	string `json:"term"`
}

type suggestions struct {
	ES		[]singleSuggestion `json:"ES"`
	CUI		[]singleSuggestion `json:"CUI"`
}

type errorMessage struct {
	Message string `json:"message"`
}

type doc struct {
	Title			string `json:"title"`
	Abstract		string `json:"abstract"`
	MeshHeadings	[]string `json:"mesh_headings"`
}

type cui struct {
	CandidateCUI	string `json:"CandidateCUI"`
}

type By func(s1, s2 *singleSuggestion) bool

func (by By) Sort(rankings []singleSuggestion) {
	ss := &ranker {
		suggestions: rankings,
		by: by,
	}
	sort.Sort(ss)
}

type ranker struct {
	suggestions		[]singleSuggestion
	by				func(s1, s2 *singleSuggestion) bool
}

func (s *ranker) Len() int {
	return len(s.suggestions)
}

func (s *ranker) Swap (i, j int) {
	s.suggestions[i], s.suggestions[j] = s.suggestions[j], s.suggestions[i]
}

func (s *ranker) Less (i, j int) bool {
	return s.by(&s.suggestions[i], &s.suggestions[j])
}

func (s Server) ApiKeywordSuggestor(c *gin.Context) {
	var word, sources string
	var size, pool int
	var merged bool
	es := s.Config.ES
	if w, ok := c.GetPostForm("term"); ok {
		word = w
		if word == "" {
			c.JSON(http.StatusOK, make([]string,0))
			return
		}
	} else {
		type message = []errorMessage
		var ret = message{
			{
				Message: "No word supplied",
			},
		}
		c.JSON(http.StatusBadRequest, ret)
		return
	}
	if retS, ok := c.GetPostForm("retSize"); ok {
		s, err := strconv.Atoi(retS)
		if err != nil {
			type message = []errorMessage
			var ret = message{
				{
					Message: "Invalid retSize.",
				},
			}
			c.JSON(http.StatusBadRequest, ret)
			return
		}
		size = s
	} else {
		size = es.DefaultRetSize
	}
	if poo, ok := c.GetPostForm("pool"); ok {
		p, err := strconv.Atoi(poo)
		if err != nil {
			type message = []errorMessage
			var ret = message{
				{
					Message: "Invalid pool.",
				},
			}
			c.JSON(http.StatusBadRequest, ret)
			return
		}
		pool = p
	} else {
		pool = es.DefaultPool
	}
	if merg, ok := c.GetPostForm("merged"); ok {
		m, err := strconv.ParseBool(merg)
		if err != nil {
			type message = []errorMessage
			var ret = message{
				{
					Message: "Invalid merge flag.",
				},
			}
			c.JSON(http.StatusBadRequest, ret)
			return
		}
		merged = m
	} else {
		merged = es.Merged
	}
	if sour, ok := c.GetPostForm("sources"); ok {
		sources = sour
	} else {
		sources = es.Sources
	}
	var ret = s.getWordSuggestion(word, size, pool, merged, sources)

	c.JSON(http.StatusOK, ret)
}

func (s Server) getWordSuggestion(word string, size int, pool int, merged bool, sources string) suggestions {
	var ret = suggestions{
		ES: make([]singleSuggestion, 0),
		CUI: make([]singleSuggestion, 0),
	}
	splitedSource := strings.Split(sources, ",")
	for _, source := range splitedSource {
		if strings.EqualFold(source, "ES") {
			esRes := s.getESWordRanking(word, size, pool)
			ret.ES = esRes
		} else if strings.EqualFold(source, "CUI") {
			cuiRes := s.getCUIWordRanking(word, size)
			ret.CUI = cuiRes
		} else {
			var ES = make([]singleSuggestion, 0)
			var CUI = make([]singleSuggestion, 0)
			ret = suggestions{
				ES,
				CUI,
			}
		}
	}
	if merged && len(splitedSource) > 1 {
		var normalizedScoreRes = minMax(ret, size)
		return normalizedScoreRes
	} else {
		return ret
	}
}

//TODO FINISH THIS
func minMax(res suggestions, size int) suggestions {
	var ret suggestions
	return ret
}

func (s Server) getESWordRanking(word string, size int, pool int) []singleSuggestion {
	var ret []singleSuggestion
	c := s.Config.ES
	username := c.Username
	secret := c.Secret
	preurl := c.URL
	indexName := c.IndexName

	if pool == 0 {
		pool = c.DefaultPool
	} else if pool > c.MaxPool {
		pool = c.MaxPool
	}

	client, err := elastic.NewSimpleClient(
		elastic.SetURL(preurl),
		elastic.SetBasicAuth(username, secret))

	if err != nil {
		log.Fatal(err)
	}

	client.Start()
	query := elastic.NewQueryStringQuery(word)

	result, err := client.Search().
		Index(indexName).Query(query).
		Sort("_score", false).
		From(0).
		Size(pool).
		Pretty(true).
		Do(context.Background())

	if err != nil {
		log.Fatal(err)
	}

	var t doc
	var count = 0
	var res []string
	var word1Count = result.Hits.TotalHits.Value

	if word1Count > 0 && count < pool {
		var allTerms []string
		for _, hit := range result.Hits.Hits {
			err := json.Unmarshal(hit.Source, &t)
			if err != nil {
				log.Fatal(err)
			}

			reg, err := regexp.Compile("[^a-zA-Z0-9-]+")
			if err != nil {
				log.Fatal(err)
			}

			procTitle := reg.ReplaceAllString(t.Title, " ")
			procAbs := reg.ReplaceAllString(t.Abstract, " ")

			procTitle = strings.ToLower(procTitle)
			procAbs = strings.ToLower(procAbs)

			var meshHeadings []string

			for _, item := range t.MeshHeadings {
				meshHeadings = append(meshHeadings, strings.ToLower(item))
			}

			splitedTitle := strings.Split(strings.Trim(procTitle, " "), " ")
			splitedAbs := strings.Split(strings.Trim(procAbs, " "), " ")

			allTerms = append(allTerms, splitedTitle...)
			allTerms = append(allTerms, splitedAbs...)
			allTerms = append(allTerms, meshHeadings...)

			count = count + 1
		}

		for ind, item := range allTerms {
			allTerms[ind] = stopwords.CleanString(item, "en", false)
			allTerms[ind] = strings.Trim(allTerms[ind], " ")
		}

		for ind, item := range allTerms {
			if item == "" {
				allTerms = append(allTerms[:ind], allTerms[ind+1:]...)
			}
		}

		encountered := map[string]bool{}

		for v:= range allTerms {
			encountered[allTerms[v]] = true
		}

		for key := range encountered {
			res = append(res, key)
		}
	}

	defer client.Stop()

	for _, term := range res {
		singleRanking := s.pmiSimilarity(float64(word1Count), word, term)
		ret = append(ret, singleRanking)
	}

	rankerScore := func(s1, s2 *singleSuggestion) bool {
		return s1.Score > s2.Score
	}

	By(rankerScore).Sort(ret)

	if size == 0 {
		size = c.DefaultRetSize
	} else if size > c.MaxRetSize {
		size = c.MaxRetSize
	}

	var returned []singleSuggestion
	var retCount = 0
	for _, k := range ret {
		if retCount < size {
			retCount = retCount + 1
			returned = append(returned, k)
		}
	}

	return returned
}

func (s Server) getCUIWordRanking(word string, size int) []singleSuggestion {
	var ret []singleSuggestion
	var wordCUI string
	preurl := s.Config.ES.MetaMap

	response, err := http.PostForm(preurl, url.Values{
		word: {""},
	})

	if err != nil {
		log.Fatal(err)
	}

	defer response.Body.Close()
	byteBody, _ := ioutil.ReadAll(response.Body)

	var cuis []cui

	err = json.Unmarshal(byteBody, &cuis)

	if err != nil {
		log.Fatal(err)
	}

	if len(cuis) > 0 {
		wordCUI = cuis[0].CandidateCUI
	} else {
		wordCUI = ""
	}

	if size == 0 {
		size = s.Config.ES.DefaultRetSize
	} else if size > s.Config.ES.MaxRetSize {
		size = s.Config.ES.MaxRetSize
	}

	intWordCUI := 0

	if wordCUI != "" {
		intWordCUI = cui2int(wordCUI)
	}

	fmt.Println(intWordCUI)

	cuiDistanceFile := s.Config.Options.Cui2VecEmbeddings
	//TODO FINISH THIS
	readCuiDistance(cuiDistanceFile)

	return ret
}

//TODO FINISH THIS
func readCuiDistance(cuiDistanceFile string) {

}

func cui2int(cui string) int {
	temp := strings.ReplaceAll(cui, "C", "")
	t := strings.ReplaceAll(temp, "c", "")
	res, _ := strconv.Atoi(t)
	return res
}

func (s Server) pmiSimilarity(word1Count float64, word1 string, word2 string) singleSuggestion {
	c := s.Config.ES
	username := c.Username
	secret := c.Secret
	preurl := c.URL
	indexName := c.IndexName

	client, err := elastic.NewSimpleClient(
		elastic.SetURL(preurl),
		elastic.SetBasicAuth(username, secret))

	if err != nil {
		log.Fatal(err)
	}

	client.Start()

	query2 := elastic.NewQueryStringQuery(word2)
	query3 := elastic.NewQueryStringQuery("(" + word1 + ")AND(" + word2 + ")")

	result1, err := client.Search().
		Index(indexName).
		Sort("_score", false).
		Do(context.Background())

	if err != nil {
		log.Fatal(err)
	}

	totalCount := result1.Hits.TotalHits.Value

	result2, err := client.Search().
		Index(indexName).Query(query2).
		Sort("_score", false).
		Do(context.Background())

	if err != nil {
		log.Fatal(err)
	}

	word2Count := result2.Hits.TotalHits.Value

	result3, err := client.Search().
		Index(indexName).Query(query3).
		Sort("_score", false).
		Do(context.Background())

	if err != nil {
		log.Fatal(err)
	}

	combinedCount := result3.Hits.TotalHits.Value

	score := calculateSimilarity(float64(totalCount), word1Count, float64(word2Count), float64(combinedCount))

	res := singleSuggestion{
		Term:  word2,
		Score: toFixed(score, 4),
	}

	return res
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num * output)) / output
}

func pmi(x float64, y float64, xy float64) float64 {
	return math.Log2((xy / x) / y)
}

func npmi(xy float64, pmiScore float64) float64 {
	return pmiScore / math.Log2(xy)
}

func calculateSimilarity(documentCount float64, f1 float64, f2 float64, f12 float64) float64 {
	var x = (f1 + 1) / (documentCount + 1)
	var y = (f2 + 1) / (documentCount + 1)
	var xy = (f12 + 1) / (documentCount + 1)
	return npmi(xy, pmi(x, y, xy))
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
