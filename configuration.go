package searchrefiner

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/stats"
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

type Config struct {
	Host       string
	AdminEmail string
	Admins     []string
	Entrez     EntrezConfig
	Options    map[string]interface{}
}

type Query struct {
	Time        time.Time `csv:"time"`
	QueryString string    `csv:"query"`
	Language    string    `csv:"language"`
	NumRet      int64     `csv:"num_ret"`
	NumRelRet   int64     `csv:"num_rel_ret"`
	Relevant    []string  `csv:"relevant"`
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
	Entrez   stats.EntrezStatisticsSource
	Plugins  []InternalPluginDetails
	Storage  map[string]*PluginStorage
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
}

// InternalPluginDetails contains details about a plugin which are vital in rendering the plugin page.
type InternalPluginDetails struct {
	URL string
	PluginDetails
}

// TemplatePlugin is the template method which will include searchrefiner components.
func TemplatePlugin(p string) template.Template {
	_, f := path.Split(p)
	return *template.Must(template.New(f).ParseFiles(append(append(PluginTemplates, Components...), p)...))
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
