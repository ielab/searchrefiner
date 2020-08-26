package searchrefiner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	rake "github.com/afjoseph/RAKE.Go"
	"github.com/gin-gonic/gin"
	"github.com/hscells/cqr"
	"github.com/hscells/guru"
	"github.com/hscells/transmute"
	"github.com/hscells/transmute/fields"
	tpipeline "github.com/hscells/transmute/pipeline"
	"github.com/ielab/toolexchange"
	"github.com/olivere/elastic"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type searchResponse struct {
	Start            int
	TotalHits        int64
	RelRet           float64
	TookInMillis     float64
	QueryString      string
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

	Plugins     []InternalPluginDetails
	PluginTitle string
}

type suggestion struct {
	Score  float64 `json:"score"`
	Term   string  `json:"term"`
	Source string  `json:"source"`
}

type suggestions struct {
	ES  []suggestion `json:"Services"`
	CUI []suggestion `json:"CUI"`
}

type doc struct {
	Title        string   `json:"title"`
	Abstract     string   `json:"abstract"`
	MeshHeadings []string `json:"mesh_headings"`
}

func (s Server) ApiKeywordSuggestor(c *gin.Context) {
	var word string

	es := s.Config.Services
	size := es.DefaultRetSize
	pool := es.DefaultPool
	merged := es.Merged
	sources := es.Sources

	if w, ok := c.GetPostForm("term"); ok {
		word = w
		if word == "" {
			c.JSON(http.StatusOK, make([]string, 0))
			return
		}
	} else {
		c.Status(http.StatusBadRequest)
		return
	}

	if retS, ok := c.GetPostForm("retSize"); ok {
		s, err := strconv.Atoi(retS)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		size = s
	}

	if poo, ok := c.GetPostForm("pool"); ok {
		p, err := strconv.Atoi(poo)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		pool = p
	}

	if merg, ok := c.GetPostForm("merged"); ok {
		m, err := strconv.ParseBool(merg)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		merged = m
	}

	if sour, ok := c.GetPostForm("sources"); ok {
		sources = sour
	}
	splitedSource := strings.Split(sources, ",")

	if merged && len(splitedSource) > 1 {
		c.JSON(http.StatusOK, s.getsuggestion(word, size, splitedSource, pool))
		return
	}

	c.JSON(http.StatusOK, s.getWordSuggestion(word, size, splitedSource, pool))
	return
}

func (s Server) getsuggestion(word string, size int, splitedSource []string, pool int) []suggestion {
	var esRes []suggestion
	var cuiRes []suggestion

	for _, source := range splitedSource {
		if strings.EqualFold(source, "Services") {
			esRes = s.getESWordRanking(word, size, pool)
		} else if strings.EqualFold(source, "CUI") {
			cuiRes = s.getCUIWordRanking(word, size)
		}
	}

	var normalizedScoreRes = minMax(esRes, cuiRes, size)

	return normalizedScoreRes
}

func (s Server) getWordSuggestion(word string, size int, splitedSource []string, pool int) suggestions {
	var ret = suggestions{}

	for _, source := range splitedSource {
		if strings.EqualFold(source, "Services") {
			esRes := s.getESWordRanking(word, size, pool)
			ret.ES = esRes
		} else if strings.EqualFold(source, "CUI") {
			cuiRes := s.getCUIWordRanking(word, size)
			ret.CUI = cuiRes
		}
	}
	return ret
}

func minMax(esRes []suggestion, cuiRes []suggestion, size int) []suggestion {
	var ret []suggestion
	var tempRet []suggestion
	var sortedTempRet []suggestion

	if len(esRes) == 0 {
		return cuiRes
	}
	if len(cuiRes) == 0 {
		return esRes
	}

	esMax, esMin := findMaxAndMin(esRes)
	cuiMax, cuiMin := findMaxAndMin(cuiRes)

	for _, esItem := range esRes {
		var singleESRes suggestion
		singleESRes.Score = (esItem.Score - esMin.Score) / (esMax.Score - esMin.Score)
		singleESRes.Source = "Services"
		singleESRes.Term = esItem.Term
		tempRet = append(tempRet, singleESRes)
	}

	for _, cuiItem := range cuiRes {
		var singleCUIRes suggestion
		singleCUIRes.Score = (cuiItem.Score - cuiMin.Score) / (cuiMax.Score - cuiMin.Score)
		singleCUIRes.Source = "CUI"
		singleCUIRes.Term = cuiItem.Term
		tempRet = append(tempRet, singleCUIRes)
	}

	scores := make([]float64, 0, len(tempRet))

	for _, v := range tempRet {
		found := false
		for _, item := range scores {
			if item == v.Score {
				found = true
			}
		}
		if !found {
			scores = append(scores, v.Score)
		}
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i] > scores[j]
	})

	for _, f := range scores {
		for _, k := range tempRet {
			if k.Score == f {
				sortedTempRet = append(sortedTempRet, k)
			}
		}
	}

	if len(sortedTempRet) < size {
		ret = sortedTempRet
	} else {
		var count = 0
		for _, val := range sortedTempRet {
			if count < size {
				ret = append(ret, val)
				count = count + 1
			}
		}
	}

	return ret
}

func findMaxAndMin(res []suggestion) (suggestion, suggestion) {
	var min = res[0]
	var max = res[0]

	for _, re := range res {
		if re.Score > max.Score {
			max = re
		}
		if re.Score < min.Score {
			min = re
		}
	}
	return max, min
}

func (s Server) getESWordRanking(word string, size int, pool int) []suggestion {
	c := s.Config.Services
	username := c.ElasticsearchPubMedUsername
	secret := c.ElasticsearchPubMedPassword
	preurl := c.ElasticsearchPubMedURL
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
		panic(err)
	}

	result, err := client.Search().
		Index(indexName).Query(elastic.NewQueryStringQuery(word)).
		Sort("_score", false).
		From(0).
		Size(pool).
		Pretty(true).
		Do(context.Background())
	if err != nil {
		panic(err)
	}

	var t doc
	var count = 0
	var res []string
	var word1Count = result.Hits.TotalHits.Value
	encountered := map[string]struct{}{}

	if word1Count > 0 && count < pool {
		var allTerms []string
		for _, hit := range result.Hits.Hits {
			err := json.Unmarshal(hit.Source, &t)
			if err != nil {
				panic(err)
			}

			title := strings.ToLower(t.Title)
			abs := strings.ToLower(t.Abstract)

			reg := regexp.MustCompile("[^a-zA-Z0-9-]+")

			// "a b  c"
			// {"a", "b", "", "c"}
			//splitTitle := strings.Split(title, " ")
			//splitAbs := strings.Split(abs, " ")
			pairs := rake.RunRake(strings.Join([]string{title, abs}, ". "))
			split := make([]string, len(pairs))
			for i, pair := range pairs {
				split[i] = pair.Key
				fmt.Println(pair.Key, pair.Value)
			}

			var meshHeadings []string
			for _, item := range t.MeshHeadings {
				meshHeadings = append(meshHeadings, strings.ToLower(item))
			}

			tokens := append(split, meshHeadings...)

			for _, k := range tokens {
				if len(k) >= 2 {
					allTerms = append(allTerms, reg.ReplaceAllString(strings.TrimSpace(k), " "))
				}
			}
			count = count + 1
		}

		for _, v := range allTerms {
			encountered[v] = struct{}{}
		}
		for key := range encountered {
			res = append(res, key)
		}
	}

	collectionSize, err := client.Count(indexName).Do(context.Background())
	if err != nil {
		panic(err)
	}

	var ret []suggestion
	for _, term := range res {
		singleRanking := s.pmiSimilarity(float64(word1Count), word, term, client, float64(collectionSize))
		ret = append(ret, singleRanking)
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Score > ret[j].Score
	})

	size = c.DefaultRetSize
	if size == 0 {

	} else if size > c.MaxRetSize {
		size = c.MaxRetSize
	}

	var returned []suggestion
	var retCount = 0
	for _, k := range ret {
		if retCount < size {
			retCount = retCount + 1
			returned = append(returned, k)
		}
	}

	return returned
}

func (s Server) getCUIWordRanking(word string, size int) []suggestion {
	var ret []suggestion

	candidates, err := s.MetaMapClient.Candidates(word)
	if err != nil {
		panic(err)
	}
	if len(candidates) == 0 {
		return ret
	}

	if size == 0 {
		size = s.Config.Services.DefaultRetSize
	} else if size > s.Config.Services.MaxRetSize {
		size = s.Config.Services.MaxRetSize
	}

	similarCUIs, err := s.CUIEmbeddings.Similar(candidates[0].CandidateCUI)
	if err != nil {
		panic(err)
	}

	if len(similarCUIs) == 0 {
		return ret
	}

	var term string
	for _, item := range similarCUIs {
		cui := item.CUI
		score := item.Value
		if val, ok := s.CUIMapping[cui]; ok {
			term = val
			oneCUI := suggestion{
				Score: score,
				Term:  term,
			}
			ret = append(ret, oneCUI)
		}
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Score > ret[j].Score
	})

	if len(ret) < size {
		return ret
	}
	return ret[:size]
}

func (s Server) pmiSimilarity(word1Count float64, word1 string, word2 string, client *elastic.Client, collectionSize float64) suggestion {
	c := s.Config.Services
	indexName := c.IndexName

	fmt.Println(word1, word2)

	query2 := elastic.NewQueryStringQuery(word2)
	//query3 := elastic.NewQueryStringQuery("(" + word1 + ") AND (" + word2 + ")")
	//query2 := elastic.NewMatchQuery("_all", word2)
	query3 := elastic.NewBoolQuery().Must(elastic.NewQueryStringQuery(word1), elastic.NewQueryStringQuery(word2))

	result2, err := client.Count(indexName).Query(query2).Do(context.Background())
	if err != nil {
		panic(err)
	}

	result3, err := client.Count(indexName).Query(query3).Do(context.Background())
	if err != nil {
		panic(err)
	}

	score := calculateSimilarity(collectionSize, word1Count, float64(result2), float64(result3))
	fmt.Println(word1, len(word1), word2, len(word2), collectionSize, word1Count, result2, result3, score)

	res := suggestion{
		Term:  word2,
		Score: score,
	}

	return res
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
	return -npmi(xy, pmi(x, y, xy))
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
func (s Server) ApiRequestTokenFromExchangeServer(query string) string {

	body := &toolexchange.Item{
		Data: map[string]string{
			"query": query,
		},
		Referrer: "searchrefiner",
	}

	reqBody, err1 := json.Marshal(body)

	if err1 != nil {
		panic(err1)
	}

	resp, err2 := http.Post(s.Config.ExchangeServerAddress, "application/json", bytes.NewBuffer(reqBody))

	if err2 != nil {
		panic(err2)
	}

	defer resp.Body.Close()

	content, err3 := ioutil.ReadAll(resp.Body)

	if err3 != nil {
		panic(err3)
	}

	return string(content)
}

func (s Server) ApiGetQuerySeedFromExchangeServer(token string) (toolexchange.Item, error) {
	var content toolexchange.Item
	path := s.Config.ExchangeServerAddress + "?token=" + token
	resp, err := http.Get(path)

	if err != nil {
		return content, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return content, err
	}

	err = json.Unmarshal(body, &content)
	if err != nil {
		return content, err
	}

	return content, nil
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
