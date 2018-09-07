package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-errors/errors"
	"github.com/hscells/cqr"
	"github.com/hscells/cui2vec"
	"github.com/hscells/groove/analysis"
	"github.com/hscells/groove/analysis/preqpp"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/learning"
	"github.com/hscells/quickumlsrest"
	"github.com/hscells/transmute"
	"github.com/hscells/transmute/pipeline"
	"github.com/ielab/searchrefiner"
	"github.com/peterbourgon/diskv"
	"log"
	"net/http"
	"os"
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
	Query       learning.CandidateQuery
	Chain       []link
	Language    string
	RawQuery    string
	Description string
}

type link struct {
	NumRet      int
	RelRet      int
	NumRel      int
	Query       string
	Description string
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
	// Load cui2vec components.
	if vector == nil || mapping == nil {
		err := initiate()
		if err != nil {
			err := errors.New("could not initiate cui2vec components")
			c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
	}

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
			c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		rep, err := bq.Representation()
		if err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		q := rep.(cqr.CommonQueryRepresentation)
		cq = learning.NewCandidateQuery(q, "1", nil)
		numret, relret, err := ret(q, s, u)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		chain[u] = append(chain[u], link{Query: query, NumRet: numret, RelRet: relret})
	}

	// If no query has been sent to the server, just render the page.
	if cq.Query == nil {
		c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/chain.html"), templating{Query: queries[u], Language: lang}))
		return
	}

	// Get a list of transformations selected by the user.
	t, ok := c.GetPostFormArray("transformations")
	if !ok {
		// If the user selected none, then just use all of them.
		t = []string{"clause_removal", "cui2vec_expansion", "mesh_parent",
			"field_restrictions", "logical_operator"}
	}

	log.Println(t)

	// This is the mapping of selected transformations to the actual transformation implementation.
	transformations := map[string]learning.Transformation{
		"clause_removal":     learning.NewClauseRemovalTransformer(),
		"cui2vec_expansion":  learning.Newcui2vecExpansionTransformer(vector, mapping, quickumls),
		"mesh_parent":        learning.NewMeshParentTransformer(),
		"mesh_explosion":     learning.NewMeSHExplosionTransformer(),
		"field_restrictions": learning.NewFieldRestrictionsTransformer(),
		"logical_operator":   learning.NewLogicalOperatorTransformer(),
	}

	// Here the transformations are filtered to just the ones that have been selected.
	selectedTransformations := make([]learning.Transformation, len(t))
	for i, transformation := range t {
		if v, ok := transformations[transformation]; ok {
			selectedTransformations[i] = v
		} else {
			err := errors.New(fmt.Sprintf("%s is not a valid transformation", transformation))
			c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
	}

	transformationType := []string{
		"Logical Operator Replacement",
		"Adjacency Range",
		"MeSH Explosion",
		"Field Restrictions",
		"Adjacency Replacement",
		"Clause Removal",
		"cui2vec Expansion",
		"MeSH Parent",
	}

	// Generate variations.
	// Only the transformations which the user has selected will be used in the variation generation.
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
		selectedTransformations...,
	)
	if err != nil && err != combinator.ErrCacheMiss {
		// Return a 500 error for now.
		c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	log.Printf("generated %d variations\n", len(candidates))

	// Select the best query.
	mu.Lock()
	defer mu.Unlock()
	nq, _, err := selector.Select(cq, candidates)
	if err != nil && err != combinator.ErrCacheMiss {
		// Return a 500 error for now.
		c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	log.Println("selected using transformation", nq.TransformationID)

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
		c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	description := fmt.Sprintf("This query was selected from %d variations. The transformation that was applied to this query was %s.", len(candidates), transformationType[nq.TransformationID])

	chain[u] = append(chain[u], link{
		Query:       q,
		NumRet:      numret,
		RelRet:      relret,
		NumRel:      len(s.Settings[u].Relevant),
		Description: description,
	})

	// Respond to a regular request.
	c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/chain.html"), templating{
		Query:       nq,
		Language:    lang,
		Chain:       chain[u],
		RawQuery:    q,
		Description: description,
	}))
	return
}

func (ChainPlugin) PermissionType() searchrefiner.PluginPermission {
	return searchrefiner.PluginUser
}

func (ChainPlugin) Details() searchrefiner.PluginDetails {
	return searchrefiner.PluginDetails{
		Title:       "Query Chain Transformer",
		Description: "Refine Boolean queries with automatic query transformations.",
		Author:      "ielab",
		Version:     "06.Sep.2018",
		ProjectURL:  "https://ielab.io/searchrefiner/",
	}
}

var Chain ChainPlugin
