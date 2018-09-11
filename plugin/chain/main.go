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
	"html/template"
	"github.com/hscells/groove"
)

type ChainPlugin struct{}

var (
	quickrank string
	quickumls quickumlsrest.Client
	vector    cui2vec.Embeddings
	mapping   cui2vec.Mapping
	queries   = make(map[string]learning.CandidateQuery)
	chain     = make(map[string][]link)
	// Cache for the statistics of the query performance predictors.
	statisticsCache = diskv.New(diskv.Options{
		BasePath:     "statistics_cache",
		Transform:    combinator.BlockTransform(8),
		CacheSizeMax: 4096 * 1024,
		Compression:  diskv.NewGzipCompression(),
	})

	models = map[string]string{
		"precision": "dart_precision.xml",
		"recall":    "dart_recall.xml",
		"f1":        "dart_f1.xml",
	}

	transformationType = []string{
		"Logical Operator Replacement",
		"Adjacency Range",
		"MeSH Explosion",
		"Field Restrictions",
		"Adjacency Replacement",
		"Clause Removal",
		"cui2vec Expansion",
		"MeSH Parent",
	}

	transformationDescriptions = []string{
		"Modifying the logical operators (AND/OR) of a single clause.",
		"",
		"Toggle explosion (tree subsumption) on a single MeSH term.",
		"Adding or removing fields from a single term.",
		"",
		"Query reduction by removal of a single clause from a query.",
		"Query expansion using CUI word embeddings.",
		"Transform a single MeSH term by rewriting it as the parent term in the ontology.",
	}

	workQueue = make(chan workRequest)
	workMu    sync.Mutex
	workMap   = make(map[string]workResponse)
)

type templating struct {
	Query       learning.CandidateQuery
	Chain       []link
	Language    string
	RawQuery    string
	Description template.HTML
	Error       error
}

type link struct {
	NumRet      int
	RelRet      int
	NumRel      int
	Query       string
	Description template.HTML
}

type workRequest struct {
	user            string
	model           string
	rawQuery        string
	lang            string
	server          searchrefiner.Server
	cq              learning.CandidateQuery
	selector        learning.QuickRankQueryCandidateSelector
	transformations []learning.Transformation
}

type workResponse struct {
	nq         learning.CandidateQuery
	candidates []learning.CandidateQuery
	err        error
	done       bool
	request    workRequest
	link       link
}

func ret(q cqr.CommonQueryRepresentation, s searchrefiner.Server, u string) (int, int, error) {
	t, _, err := combinator.NewLogicalTree(groove.NewPipelineQuery("0", "0", q), s.Entrez, searchrefiner.QueryCacher)
	if err != nil {
		log.Println(err)
		return 0, 0, err
	}
	d := t.Documents(searchrefiner.QueryCacher)
	foundRel := 0
	for _, doc := range d {
		for _, rel := range s.Settings[u].Relevant {
			if doc == rel {
				foundRel++
			}
		}
	}
	return len(d), foundRel, nil
}

func initiate() error {
	doWork()

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

func doWork() {
	go func() {
		for {
			select {
			case w := <-workQueue:
				log.Println("recieved work request")

				// generate candidates and select the best one.
				nq, candidates, err := transform(w.cq, w.selector, w.transformations...)

				nqnumret, nqrelret, err := ret(nq.Query, w.server, w.user)
				if err != nil {
					log.Println(err)
					workMap[w.user] = workResponse{
						err:  err,
						done: true,
					}
					break
				}

				numrel := len(w.server.Settings[w.user].Relevant)

				description := fmt.Sprintf(`
This query was selected from %d variations. 
The transformation that was applied to this query was <span class="tooltip label label-rounded c-hand" data-tooltip="%s">%s</span>. 
The optimisation that was applied was %s.`, len(candidates), transformationDescriptions[nq.TransformationID], transformationType[nq.TransformationID], w.model)

				var q string
				switch w.lang {
				case "pubmed":
					q, _ = transmute.CompileCqr2PubMed(nq.Query)
				default:
					q, _ = transmute.CompileCqr2Medline(nq.Query)
				}

				workMu.Lock()
				queries[w.user] = nq
				if len(chain[w.user]) == 0 {
					cqnumret, cqrelret, err := ret(w.cq.Query, w.server, w.user)
					if err != nil {
						log.Println(err)
						workMap[w.user] = workResponse{
							err:  err,
							done: true,
						}
						break
					}
					chain[w.user] = append(chain[w.user], link{
						Query:       w.rawQuery,
						NumRet:      cqnumret,
						NumRel:      numrel,
						RelRet:      cqrelret,
						Description: template.HTML("This is the original query that was submitted."),
					})
				}
				chain[w.user] = append(chain[w.user], link{
					NumRet:      nqnumret,
					NumRel:      numrel,
					RelRet:      nqrelret,
					Query:       q,
					Description: template.HTML(description),
				})
				workMap[w.user] = workResponse{
					nq:         nq,
					candidates: candidates,
					err:        err,
					done:       true,
					request:    w,
					link: link{
						NumRet:      nqnumret,
						NumRel:      numrel,
						RelRet:      nqrelret,
						Query:       q,
						Description: template.HTML(description),
					},
				}
				workMu.Unlock()
				log.Println("sending work response")
			}
		}
	}()
}

func transform(cq learning.CandidateQuery, selector learning.QuickRankQueryCandidateSelector, selectedTransformations ...learning.Transformation) (learning.CandidateQuery, []learning.CandidateQuery, error) {
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
	if err != nil {
		return cq, nil, err
	}

	log.Printf("generated %d variations\n", len(candidates))

	// Select the best query.
	nq, _, err := selector.Select(cq, candidates)
	if err != nil {
		return cq, nil, err
	}
	log.Println("selected using transformation", nq.TransformationID)
	return nq, candidates, nil
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

	// This is the mapping of selected transformations to the actual transformation implementation.
	transformations := map[string]learning.Transformation{
		"clause_removal":     learning.NewClauseRemovalTransformer(),
		"cui2vec_expansion":  learning.Newcui2vecExpansionTransformer(vector, mapping, quickumls),
		"mesh_parent":        learning.NewMeshParentTransformer(),
		"mesh_explosion":     learning.NewMeSHExplosionTransformer(),
		"field_restrictions": learning.NewFieldRestrictionsTransformer(),
		"logical_operator":   learning.NewLogicalOperatorTransformer(),
	}

	// Grab the username of the logged in user.
	u := s.UserState.Username(c.Request)

	// Create an entry in the query expansion map for the user.
	if _, ok := queries[u]; !ok {
		log.Println("making new query for user")
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
		c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/index.html"), templating{Query: queries[u], Language: lang}))
		return
	}

	var model string
	model, ok = c.GetPostForm("model")
	if !ok {
		model = "precision"
	}

	// selector is a quickrank candidate selector configured to only select to a depth of one.
	selector := learning.NewQuickRankQueryCandidateSelector(
		quickrank,
		map[string]interface{}{
			"model-in":    fmt.Sprintf("plugin/chain/%s", models[model]),
			"test-metric": "DCG",
			"test-cutoff": 1,
			"scores":      "scores.txt",
		},
		learning.QuickRankCandidateSelectorMaxDepth(1),
	)

	// Respond to a request to expand a brand new query.
	var query string
	if query, ok = c.GetPostForm("query"); ok {
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
		cq = learning.NewCandidateQuery(q, "", nil)
	}

	// Get a list of transformations selected by the user.
	t, ok := c.GetPostFormArray("transformations")
	if !ok {
		// If the user selected none, then just use all of them.
		t = []string{"clause_removal", "cui2vec_expansion", "mesh_parent", "field_restrictions", "logical_operator"}
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

	var (
		nq       learning.CandidateQuery
		response workResponse
	)

	workMu.Lock()
	defer workMu.Unlock()
	if response, ok = workMap[u]; !ok && cq.Query != nil && c.Request.Method == "POST" { // If no work exists, create a job.
		log.Println("sending work request")
		workMap[u] = workResponse{
			done: false,
		}
		work := workRequest{
			user:            u,
			cq:              cq,
			selector:        selector,
			transformations: selectedTransformations,
			model:           model,
			rawQuery:        query,
			lang:            lang,
		}
		workQueue <- work
		c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/queue.html"), nil))
		return
	} else { // Otherwise, we can either get the results, or continue to wait until the request is fulfilled.
		if c.Request.Method != "GET" {
			log.Println("only responding to GET requests")
			c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/queue.html"), nil))
			return
		}
		if response.done {
			log.Println("completed work")
			if response.err != nil {
				log.Println(response.err)
				c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: response.err.Error(), BackLink: "/plugin/chain"})
				c.AbortWithError(http.StatusInternalServerError, response.err)
				return
			}
		} else if ok && cq.Query != nil {
			log.Println("work has not been completed")
			c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/queue.html"), nil))
			return
		} else {
			log.Println("there is no work and no query has been submitted")
			if len(chain[u]) > 0 {
				c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/index.html"), templating{Query: queries[u], Language: lang, Chain: chain[u], Description: chain[u][len(chain[u])-1].Description, RawQuery: chain[u][len(chain[u])-1].Query, Error: response.err}))
			} else {
				c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/index.html"), templating{Language: lang, Error: response.err}))
			}
			return
		}
	}

	// Only now can we be sure that there is no more work for this user and we can delete the job.
	delete(workMap, u)

	// This ensures we only even look at 5 features in the past maximum.
	// This prevents quickrank from being overloaded with features and crashing.
	if len(nq.Chain) > 5 {
		nq.Chain = nq.Chain[1:]
	}

	// Respond to a regular request.
	c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/index.html"), templating{
		Query:       nq,
		Language:    lang,
		Chain:       chain[u],
		RawQuery:    response.link.Query,
		Description: template.HTML(response.link.Description),
		Error:       response.err,
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
