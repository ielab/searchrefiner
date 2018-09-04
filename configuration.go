package searchrefiner

import (
	"github.com/xyproto/pinterface"
	"github.com/hscells/groove/stats"
	"github.com/hscells/groove/combinator"
	"github.com/gin-gonic/gin"
	"html/template"
	"path"
	"github.com/gin-gonic/gin/render"
)

var (
	seen                = combinator.NewFileQueryCache("file_cache")
	Components          = []string{"components/sidebar.tmpl.html", "components/util.tmpl.html", "components/login.template.html", "components/bigbro.tmpl.html"}
	ServerConfiguration = Server{}
)

type PluginPermission int

const (
	PluginAdmin  = iota
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
	UserState pinterface.IUserState
	Perm      pinterface.IPermissions
	Queries   map[string][]Query
	Settings  map[string]Settings
	Config    Config
	Entrez    stats.EntrezStatisticsSource
}

type Plugin interface {
	Serve(Server, *gin.Context)
	PermissionType() PluginPermission
}

func TemplatePlugin(p string) template.Template {
	_, f := path.Split(p)
	return *template.Must(template.New(f).ParseFiles(append(Components, p)...))
}

func RenderPlugin(tmpl template.Template, data interface{}) render.HTML {
	return render.HTML{
		Template: &tmpl,
		Name:     tmpl.Name(),
		Data:     data,
	}
}