package searchrefiner

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/hscells/cui2vec"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/stats"
	"github.com/hscells/metawrap"
	"github.com/hscells/quickumlsrest"
	"github.com/xyproto/permissionbolt"
	"html/template"
	"path"
	"time"
)

var (
	QueryCacher         = combinator.NewFileQueryCache("file_cache")
	PluginTemplates     []string
	Components          = []string{"components/sidebar.tmpl.html", "components/util.tmpl.html", "components/login.template.html", "components/announcement.tmpl.html"}
	ServerConfiguration = Server{}
)

type PluginPermission int

const (
	PluginAdmin = iota
	PluginUser
	PluginPublic
)

type EntrezConfig struct {
	Email  string
	APIKey string
}

type Services struct {
	ElasticsearchPubMedURL      string
	ElasticsearchPubMedUsername string
	ElasticsearchPubMedPassword string
	ElasticsearchUMLSURL        string
	ElasticsearchUMLSUsername   string
	ElasticsearchUMLSPassword   string
	MetaMapURL                  string
	IndexName                   string
	DefaultPool                 int
	DefaultRetSize              int
	MaxRetSize                  int
	MaxPool                     int
	Merged                      bool
	Sources                     string
}

type OtherServiceAddresses struct {
	SRA string
}

type Config struct {
	Host                  string
	AdminEmail            string
	Admins                []string
	Entrez                EntrezConfig
	Resources             Resources // TODO: This should be merged into the Services struct.
	Mode                  string
	EnableAll             bool
	Services              Services
	ExchangeServerAddress string
	OtherServiceAddresses OtherServiceAddresses
}

type Resources struct {
	Cui2VecEmbeddings string
	Cui2VecMappings   string
	Quiche            string
	QuickRank         string
}

type Query struct {
	Time        time.Time `csv:"time"`
	QueryString string    `csv:"query"`
	Language    string    `csv:"language"`
	NumRet      int64     `csv:"num_ret"`
	NumRelRet   int64     `csv:"num_rel_ret"`
	Relevant    []string  `csv:"relevant"`

	Plugins     []InternalPluginDetails
	PluginTitle string
}

type ErrorPage struct {
	Error    string
	BackLink string
}

type Settings struct {
	Relevant combinator.Documents
}

type Server struct {
	Perm     *permissionbolt.Permissions
	Queries  map[string][]Query
	Settings map[string]Settings
	Config   Config
	Plugins  []InternalPluginDetails
	Storage  map[string]*PluginStorage

	Entrez        stats.EntrezStatisticsSource
	CUIEmbeddings *cui2vec.PrecomputedEmbeddings
	QuicheCache   quickumlsrest.Cache
	CUIMapping    cui2vec.Mapping
	MetaMapClient metawrap.HTTPClient
}

// Plugin is the interface that must be implemented in order to register an external tool.
// See more: http://ielab.io/searchrefiner/plugins/
type Plugin interface {
	Serve(Server, *gin.Context)
	PermissionType() PluginPermission
	Details() PluginDetails
}

// PluginDetails are details about a plugin which is shown in the plugins page of searchrefiner.
type PluginDetails struct {
	Title       string
	Description string
	Author      string
	Version     string
	ProjectURL  string

	AcceptsQueryPosts bool
}

// InternalPluginDetails contains details about a plugin which are vital in rendering the plugin page.
type InternalPluginDetails struct {
	URL string
	PluginDetails
}

func TmplDict(values ...interface{}) (map[string]interface{}, error) {
	// Thank-you to https://stackoverflow.com/a/18276968!
	if len(values)%2 != 0 {
		return nil, errors.New("invalid dict call")
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}

		switch v := values[i+1].(type) {
		case string:
			dict[key] = template.HTML(v)
		default:
			dict[key] = v
		}
	}
	return dict, nil
}

// TemplatePlugin is the template method which will include searchrefiner components.
func TemplatePlugin(p string) template.Template {
	_, f := path.Split(p)
	return *template.Must(template.
		New(f).
		Funcs(template.FuncMap{"dict": TmplDict}).
		ParseFiles(append(append(PluginTemplates, Components...), p)...))
}

// RenderPlugin returns a gin-compatible HTML renderer for plugins.
func RenderPlugin(tmpl template.Template, data interface{}) render.HTML {
	return render.HTML{
		Template: &tmpl,
		Name:     tmpl.Name(),
		Data:     data,
	}
}

func (s Server) getAllPluginStorage() (map[string]map[string]map[string]string, error) {
	st := make(map[string]map[string]map[string]string)
	for plugin, ps := range s.Storage {
		st[plugin] = make(map[string]map[string]string)
		buckets, err := ps.GetBuckets()
		if err != nil {
			return nil, err
		}
		for _, bucket := range buckets {
			v, err := ps.GetValues(bucket)
			if err != nil {
				return nil, err
			}
			st[plugin][bucket] = v
		}
	}
	return st, nil
}
