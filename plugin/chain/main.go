package main

import (
	"github.com/ielab/searchrefiner"
	"github.com/gin-gonic/gin"
	"net/http"
	"github.com/hscells/cqr"
	"github.com/hscells/transmute/pipeline"
	"github.com/hscells/groove/learning"
	"log"
	"github.com/go-errors/errors"
	"github.com/hscells/groove/analysis"
	"github.com/hscells/groove/analysis/preqpp"
	"fmt"
	"github.com/hscells/cui2vec"
	"os"
	"github.com/hscells/quickumlsrest"
	"github.com/hscells/transmute"
	"github.com/hscells/groove/combinator"
	"github.com/peterbourgon/diskv"
	"sync"
)

type ChainPlugin struct{}

var (
	quickrank string
	quickumls quickumlsrest.Client
	vector    cui2vec.Embeddings
	mapping   cui2vec.Mapping
	mu        sync.Mutex
	queries   = make(map[string]learning.CandidateQuery)
	chain     = make(map[string][]link)
	// Cache for the statistics of the query performance predictors.
	statisticsCache = diskv.New(diskv.Options{
		BasePath:     "statistics_cache",
		Transform:    combinator.BlockTransform(8),
		CacheSizeMax: 4096 * 1024,
		Compression:  diskv.NewGzipCompression(),
	})
)

type templating struct {
	Query    learning.CandidateQuery
	Chain    []link
	Language string
	RawQuery string
}

type link struct {
	NumRet int
	RelRet int
	Query  string
}

func ret(q cqr.CommonQueryRepresentation, s searchrefiner.Server, u string) (int, int, error) {
	eq, _ := transmute.CompileCqr2PubMed(q)
	d, err := s.Entrez.Search(eq, s.Entrez.SearchSize(100000))
	if err != nil {
		return 0, 0, err
	}
	foundRel := 0
	for _, doc := range d {
		for _, rel := range s.Settings[u].Relevant {
			if combinator.Document(doc) == rel {
				foundRel++
			}
		}
	}
	return len(d), foundRel, nil
}

func initiate() error {
	quickrank = searchrefiner.ServerConfiguration.Config.Options["QuicklearnBinary"].(string)
	quickumls = quickumlsrest.NewClient(searchrefiner.ServerConfiguration.Config.Options["QuickUMLSURL"].(string))

	log.Println("loading cui2vec components")
	f, err := os.OpenFile(searchrefiner.ServerConfiguration.Config.Options["Cui2VecEmbeddings"].(string), os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	log.Println("loading mappings")
	mapping, err = cui2vec.LoadCUIMapping(searchrefiner.ServerConfiguration.Config.Options["Cui2VecMappings"].(string))
	if err != nil {
		return err
	}

	log.Println("loading model")
	vector, err = cui2vec.LoadModel(f, true)
	if err != nil {
		return err
	}

	return nil
}

func (ChainPlugin) Serve(s searchrefiner.Server, c *gin.Context) {
	fmt.Println(vector == nil, mapping == nil)
	// Load cui2vec components.
	if vector == nil || mapping == nil {
		err := initiate()
		if err != nil {
			// Return a 500 error for now.
			log.Println(errors.New("could not initiate cui2vec"))
			c.Status(http.StatusInternalServerError)
			return
		}
	}
	fmt.Println(vector == nil, mapping == nil)

	// Grab the username of the logged in user.
	u := s.UserState.Username(c.Request)

	// Create an entry in the query expansion map for the user.
	if _, ok := queries[u]; !ok {
		fmt.Println("making new query for user")
		queries[u] = learning.CandidateQuery{}
		chain[u] = []link{}
	}

	// Set the current candidate query to the most recent candidate.
	cq := queries[u]

	// Get the query language.
	lang, ok := c.GetPostForm("lang")
	if !ok {
		lang = "medline"
	}

	// Respond to a request to clear the users queries.
	if _, ok := c.GetPostForm("clear"); ok {
		queries[u] = learning.CandidateQuery{}
		chain[u] = []link{}
		c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/chain.html"), templating{Query: queries[u], Language: lang}))
		return
	}

	model, ok := c.GetPostForm("model")
	if !ok {
		// Return a 500 error for now.
		log.Println(errors.New("no model specified"))
		c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/chain.html"), templating{Query: queries[u], Language: lang}))
		return
	}

	// selector is a quickrank candidate selector configured to only select to a depth of one.
	selector := learning.NewQuickRankQueryCandidateSelector(
		quickrank,
		map[string]interface{}{
			"model-in":    fmt.Sprintf("plugin/chain/%s", model),
			"test-metric": "DCG",
			"test-cutoff": 1,
			"scores":      "scores.txt",
		},
		learning.QuickRankCandidateSelectorMaxDepth(1),
	)

	// Respond to a request to expand a brand new query.
	if query, ok := c.GetPostForm("query"); ok {
		// Clear any existing queries.
		queries[u] = learning.CandidateQuery{}
		chain[u] = []link{}

		t := make(map[string]pipeline.TransmutePipeline)
		t["pubmed"] = transmute.Pubmed2Cqr
		t["medline"] = transmute.Medline2Cqr

		compiler := t["medline"]
		if v, ok := t[lang]; ok {
			compiler = v
		} else {
			lang = "medline"
		}

		bq, err := compiler.Execute(query)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		rep, err := bq.Representation()
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		q := rep.(cqr.CommonQueryRepresentation)
		cq = learning.NewCandidateQuery(q, "1", nil)
		numret, relret, err := ret(q, s, u)
		if err != nil {
			log.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		chain[u] = append(chain[u], link{Query: query, NumRet: numret, RelRet: relret})
	}

	if cq.Query == nil {
		c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/chain.html"), templating{Query: queries[u], Language: lang}))
		return
	}

	fmt.Println(cq.Query, lang)

	// Generate variations.
	candidates, err := learning.Variations(
		cq,
		searchrefiner.ServerConfiguration.Entrez,
		analysis.NewDiskMeasurementExecutor(statisticsCache),
		[]analysis.Measurement{
			preqpp.RetrievalSize,
			analysis.BooleanClauses,
			analysis.BooleanKeywords,
			analysis.BooleanTruncated,
			analysis.BooleanFields,
			analysis.MeshNonExplodedCount,
			analysis.MeshExplodedCount,
			analysis.MeshKeywordCount,
			analysis.MeshAvgDepth,
			analysis.MeshMaxDepth,
		},
		learning.NewClauseRemovalTransformer(),
		learning.Newcui2vecExpansionTransformer(vector, mapping, quickumls),
		learning.NewMeshParentTransformer(),
		learning.NewMeSHExplosionTransformer(),
		learning.NewFieldRestrictionsTransformer(),
		learning.NewLogicalOperatorTransformer(),
	)
	if err != nil && err != combinator.ErrCacheMiss {
		// Return a 500 error for now.
		log.Println(err)
		c.Status(http.StatusInternalServerError)
		return
	}

	log.Printf("generated %d variations\n", len(candidates))

	// Select the best query.
	mu.Lock()
	defer mu.Unlock()
	nq, _, err := selector.Select(cq, candidates)
	if err != nil && err != combinator.ErrCacheMiss {
		// Return a 500 error for now.
		log.Println(err)
		c.Status(http.StatusInternalServerError)
		return
	}

	// This ensures we only even look at 5 features in the past maximum.
	// This prevents quickrank from being overloaded with features and crashing.
	if len(nq.Chain) > 5 {
		nq.Chain = nq.Chain[1:]
	}
	fmt.Println(len(nq.Chain))
	queries[u] = nq

	var q string
	switch lang {
	case "pubmed":
		q, _ = transmute.CompileCqr2PubMed(nq.Query)
	default:
		q, _ = transmute.CompileCqr2Medline(nq.Query)
	}

	numret, relret, err := ret(nq.Query, s, u)
	if err != nil {
		log.Println(err)
		c.Status(http.StatusInternalServerError)
		return
	}
	chain[u] = append(chain[u], link{Query: q, NumRet: numret, RelRet: relret})

	// Respond to a regular request.
	c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/chain.html"), templating{Query: nq, Language: lang, Chain: chain[u], RawQuery: q}))
	return
}

func (ChainPlugin) PermissionType() searchrefiner.PluginPermission {
	return searchrefiner.PluginUser
}

var Chain ChainPlugin
