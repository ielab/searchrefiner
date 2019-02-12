package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-errors/errors"
	"github.com/google/uuid"
	"github.com/hscells/cqr"
	"github.com/hscells/cui2vec"
	"github.com/hscells/groove/analysis"
	"github.com/hscells/groove/analysis/preqpp"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/eval"
	"github.com/hscells/groove/learning"
	gpipeline "github.com/hscells/groove/pipeline"
	"github.com/hscells/quickumlsrest"
	"github.com/hscells/quickumlsrest/quiche"
	"github.com/hscells/transmute"
	tpipeline "github.com/hscells/transmute/pipeline"
	"github.com/hscells/trecresults"
	"github.com/ielab/searchrefiner"
	"github.com/peterbourgon/diskv"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
)

type ChainPlugin struct{}

var (
	quickumlsCache quickumlsrest.Cache
	vector         cui2vec.Embeddings
	mapping        cui2vec.Mapping
	queries        = make(map[string]learning.CandidateQuery)
	chain          = make(map[string][]link)
	// Cache for the statistics of the query performance predictors.
	statisticsCache = diskv.New(diskv.Options{
		BasePath:     "statistics_cache",
		Transform:    combinator.BlockTransform(8),
		CacheSizeMax: 4096 * 1024,
		Compression:  diskv.NewGzipCompression(),
	})

	models = map[string]string{
		"balanced": "breadth_evaluation_diversifiedprecision.F0.5Measure.dart.xml",
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

	workMu  sync.Mutex
	workMap = make(map[string]workResponse)
)

type templating struct {
	Query       learning.CandidateQuery
	Chain       []link
	Language    string
	RawQuery    string
	Description template.HTML
	Evaluation  map[string]float64
	Error       error
}

type link struct {
	Evaluation      map[string]float64
	Transformations map[string]bool
	Query           string
	Description     template.HTML
}

type workRequest struct {
	user                   string
	model                  string
	rawQuery               string
	lang                   string
	server                 searchrefiner.Server
	cq                     learning.CandidateQuery
	selector               learning.QueryChainCandidateSelector
	transformations        []learning.Transformation
	appliedTransformations []string
}

type workResponse struct {
	nq         learning.CandidateQuery
	candidates []learning.CandidateQuery
	err        error
	done       bool
	request    workRequest
	link       link
}

func ret(q cqr.CommonQueryRepresentation, s searchrefiner.Server, u string) (map[string]float64, error) {
	gq := gpipeline.NewQuery("0", "0", q)
	t, _, err := combinator.NewLogicalTree(gq, s.Entrez, searchrefiner.QueryCacher)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	d := t.Documents(searchrefiner.QueryCacher)
	qrels := trecresults.NewQrelsFile()
	qrels.Qrels["0"] = make(trecresults.Qrels, len(s.Settings[u].Relevant))
	for _, pmid := range s.Settings[u].Relevant {
		qrels.Qrels["0"][pmid.String()] = &trecresults.Qrel{Topic: "0", Iteration: "0", DocId: pmid.String(), Score: 1}
	}
	results := d.Results(gq, "0")
	return eval.Evaluate(
		[]eval.Evaluator{
			eval.PrecisionEvaluator,
			eval.RecallEvaluator,
			eval.F1Measure,
			eval.F05Measure,
			eval.F3Measure,
			eval.NumRel,
			eval.NumRet,
			eval.NumRelRet,
		},
		&results,
		*qrels,
		"0",
	), nil
}

func initiate() error {
	//quickrank = searchrefiner.ServerConfiguration.Config.Options["QuicklearnBinary"].(string)

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
	vector, err = cui2vec.NewPrecomputedEmbeddings(f)
	if err != nil {
		return err
	}

	log.Println("loading quiche cache")
	quickumlsCache, err = quiche.Load(searchrefiner.ServerConfiguration.Config.Options["Quiche"].(string))
	if err != nil {
		return err
	}

	log.Println("loaded all components")
	return nil
}

func (w workRequest) start() {
	go func() {
		log.Println("recieved work request")

		// generate candidates and select the best one.
		nq, candidates, err := transform(w.cq, w.selector, w.transformations...)
		if err != nil {
			log.Println(err)
			workMap[w.user] = workResponse{
				err:  err,
				done: true,
			}
			return
		}

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
		defer workMu.Unlock()
		queries[w.user] = nq
		if len(chain[w.user]) == 0 {
			evaluation, err := ret(w.cq.Query, w.server, w.user)
			if err != nil {
				log.Println(err)
				workMap[w.user] = workResponse{
					err:  err,
					done: true,
				}
				return
			}

			chain[w.user] = append(chain[w.user], link{
				Query:       w.rawQuery,
				Evaluation:  evaluation,
				Description: template.HTML("This is the original query that was submitted."),
			})
		}
		evaluation, err := ret(nq.Query, w.server, w.user)
		if err != nil {
			log.Println(err)
			workMap[w.user] = workResponse{
				err:  err,
				done: true,
			}
			return
		}
		// This is the mapping of selected transformations to the actual transformation implementation.
		transformations := map[string]bool{
			"clause_removal":     false,
			"cui2vec_expansion":  false,
			"mesh_parent":        false,
			"mesh_explosion":     false,
			"field_restrictions": false,
			"logical_operator":   false,
		}
		for _, t := range w.appliedTransformations {
			transformations[t] = true
		}
		chain[w.user] = append(chain[w.user], link{
			Query:           q,
			Evaluation:      evaluation,
			Transformations: transformations,
			Description:     template.HTML(description),
		})
		workMap[w.user] = workResponse{
			nq:         nq,
			candidates: candidates,
			err:        err,
			done:       true,
			request:    w,
			link: link{
				Query:       q,
				Description: template.HTML(description),
			},
		}
		log.Println("sending work response")
		return
	}()
}

func transform(cq learning.CandidateQuery, selector learning.QueryChainCandidateSelector, selectedTransformations ...learning.Transformation) (learning.CandidateQuery, []learning.CandidateQuery, error) {
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
			panic(err)
		}
	}

	// This is the mapping of selected transformations to the actual transformation implementation.
	transformations := map[string]learning.Transformation{
		"clause_removal":     learning.NewClauseRemovalTransformer(),
		"cui2vec_expansion":  learning.Newcui2vecExpansionTransformer(vector, mapping, quickumlsCache),
		"mesh_parent":        learning.NewMeshParentTransformer(),
		"mesh_explosion":     learning.NewMeSHExplosionTransformer(),
		"field_restrictions": learning.NewFieldRestrictionsTransformer(),
		"logical_operator":   learning.NewLogicalOperatorTransformer(),
	}

	// Grab the username of the logged in user.
	u := s.Perm.UserState().Username(c.Request)

	// Create an entry in the query expansion map for the user.
	if _, ok := queries[u]; !ok {
		log.Println("making new query for user")
		queries[u] = learning.CandidateQuery{}
		chain[u] = []link{}
	}

	// Set the current candidate query to the most recent candidate.
	cq := queries[u]
	cq.Topic = "0"

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
		model = "balanced"
	}

	// selector is a quickrank candidate selector configured to only select to a depth of one.
	selector := learning.NewQuickRankQueryCandidateSelector(
		searchrefiner.ServerConfiguration.Config.Options["QuickRank"].(string),
		map[string]interface{}{
			"model-in":    fmt.Sprintf("plugin/chain/%s", models[model]),
			"test-metric": "DCG",
			"test-cutoff": 1,
			"scores":      uuid.New().String(),
		},
		learning.QuickRankCandidateSelectorMaxDepth(1),
		learning.QuickRankCandidateSelectorStatisticsSource(s.Entrez),
	)

	// Respond to a request to expand a brand new query.
	var query string
	if query, ok = c.GetPostForm("query"); ok {
		// Clear any existing queries.
		queries[u] = learning.CandidateQuery{Topic: "0"}
		chain[u] = []link{}

		t := make(map[string]tpipeline.TransmutePipeline)
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
			return
		}

		rep, err := bq.Representation()
		if err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: err.Error(), BackLink: "/plugin/chain"})
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
			return
		}
	}

	var (
		nq       learning.CandidateQuery
		response workResponse
	)

	if response, ok = workMap[u]; !ok && cq.Query != nil && c.Request.Method == "POST" { // If no work exists, create a job.
		log.Println("sending work request")
		workMap[u] = workResponse{
			done: false,
		}
		work := workRequest{
			user:                   u,
			server:                 s,
			cq:                     cq,
			selector:               selector,
			transformations:        selectedTransformations,
			appliedTransformations: t,
			model:                  model,
			rawQuery:               query,
			lang:                   lang,
		}
		work.start()
		c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/queue.html"), nil))
		return
	} else { // Otherwise, we can either get the results, or continue to wait until the request is fulfilled.
		if response.done {
			log.Println("completed work")
			if response.err != nil {
				log.Println(response.err)
				c.HTML(http.StatusInternalServerError, "error.html", searchrefiner.ErrorPage{Error: response.err.Error(), BackLink: "/plugin/chain"})
				panic(response.err)
			}
			//c.Render(http.StatusAccepted, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/index.html"), templating{Query: queries[u], Language: lang, Chain: chain[u], Description: chain[u][len(chain[u])-1].Description, RawQuery: chain[u][len(chain[u])-1].Query, Error: response.err}))
			//return
		} else if ok && !response.done {
			log.Println("work has not been completed")
			c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/queue.html"), nil))
			return
		} else {
			if len(chain[u]) > 0 {
				log.Println("there is no work but a chain exists")
				c.Render(http.StatusOK, searchrefiner.RenderPlugin(searchrefiner.TemplatePlugin("plugin/chain/index.html"), templating{Query: queries[u], Language: lang, Chain: chain[u], Description: chain[u][len(chain[u])-1].Description, RawQuery: chain[u][len(chain[u])-1].Query, Error: response.err}))
			} else {
				log.Println("there is no work and no query has been submitted")
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
		Version:     "15.Jan.2019",
		ProjectURL:  "https://ielab.io/searchrefiner/",
	}
}

var Chain ChainPlugin
