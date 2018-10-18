package searchrefiner

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/stats"
	"github.com/xyproto/permissionbolt"
	"html/template"
	"path"
)

var (
	QueryCacher         = combinator.NewFileQueryCache("file_cache")
	Components          = []string{"components/sidebar.tmpl.html", "components/util.tmpl.html", "components/login.template.html", "components/bigbro.tmpl.html"}
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
	QueryString     string
	Language        string
	NumRet          int64
	PreviousQueries []Query
	Relevant        []string
}

type ErrorPage struct {
	Error    string
	BackLink string
}

type Settings struct {
	Relevant combinator.Documents
}

type Server struct {
	Perm      *permissionbolt.Permissions
	Queries   map[string][]Query
	Settings  map[string]Settings
	Config    Config
	Entrez    stats.EntrezStatisticsSource
	Plugins   []InternalPluginDetails
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
	return *template.Must(template.New(f).ParseFiles(append(Components, p)...))
}

// RenderPlugin returns a gin-compatible HTML renderer for plugins.
func RenderPlugin(tmpl template.Template, data interface{}) render.HTML {
	return render.HTML{
		Template: &tmpl,
		Name:     tmpl.Name(),
		Data:     data,
	}
}
