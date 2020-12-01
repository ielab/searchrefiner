package searchrefiner

import (
	"fmt"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"net/http"
)

func HandleAccountLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "account_login.html", nil)
}

func HandleAccountCreate(c *gin.Context) {
	c.HTML(http.StatusOK, "account_create.html", nil)
}

func (s Server) ApiAccountLogin(c *gin.Context) {
	var username, password string
	if v, ok := c.GetPostForm("username"); ok {
		username = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "no username supplied", BackLink: "/account/login"})
		return
	}

	if v, ok := c.GetPostForm("password"); ok {
		password = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "no password supplied", BackLink: "/account/login"})
		return
	}

	if s.Perm.UserState().CorrectPassword(username, password) {
		err := s.Perm.UserState().Login(c.Writer, username)
		if err != nil {
			c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: err.Error(), BackLink: "/account/login"})
			return
		}
		log.Info(fmt.Sprintf("[login=%s]", username))
		c.Redirect(http.StatusFound, "/")
		return
	}
	c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "invalid login credentials", BackLink: "/account/login"})
	c.Status(http.StatusUnauthorized)
	return
}

func (s Server) ApiAccountCreate(c *gin.Context) {
	var username, password, password2 string
	if v, ok := c.GetPostForm("username"); ok {
		username = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "no username supplied", BackLink: "/account/create"})
		return
	}

	if v, ok := c.GetPostForm("password"); ok {
		password = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "passwords do not match", BackLink: "/account/create"})
		return
	}

	if v, ok := c.GetPostForm("password2"); ok {
		password2 = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "passwords do not match", BackLink: "/account/create"})
		return
	}

	if password != password2 {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "passwords do not match", BackLink: "/account/create"})
		return
	}

	if s.Perm.UserState().HasUser(username) {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "a user with that name already exists", BackLink: "/account/create"})
		return
	}

	isAdmin := false
	for _, u := range s.Config.Admins {
		if u == username {
			s.Perm.UserState().AddUser(username, password, username)
			s.Perm.UserState().SetAdminStatus(username)
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		s.Perm.UserState().AddUser(username, password, username)
	}

	s.Perm.UserState().MarkConfirmed(username)
	err := s.Perm.UserState().Login(c.Writer, username)
	if err != nil {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: err.Error(), BackLink: "/account/create"})
		return
	}
	c.Redirect(http.StatusFound, "/")
	return
}

func (s Server) ApiAccountLogout(c *gin.Context) {
	username := s.Perm.UserState().Username(c.Request)
	if s.Perm.UserState().IsLoggedIn(username) {
		s.Perm.UserState().Logout(username)
		s.Perm.UserState().ClearCookie(c.Writer)
	}
	c.Redirect(http.StatusFound, "/account/login")
	return
}

func (s Server) ApiAccountUsername(c *gin.Context) {
	username := s.Perm.UserState().Username(c.Request)
	if !s.Perm.UserState().IsLoggedIn(username) {
		c.String(http.StatusOK, "anonymous")
		return
	}
	c.String(http.StatusOK, username)
	return
}

func (s Server) HandleAdmin(c *gin.Context) {
	u, err := s.Perm.UserState().AllUnconfirmedUsernames()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		return
	}

	conf, err := s.Perm.UserState().AllUsernames()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		return
	}

	storage, err := s.getAllPluginStorage()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		return
	}

	type admin struct {
		Unconfirmed []string
		Confirmed   []string
		Storage     map[string]map[string]map[string]string
	}

	c.HTML(http.StatusOK, "admin.html", admin{Unconfirmed: u, Confirmed: conf, Storage: storage})
}

func (s Server) ApiAdminConfirm(c *gin.Context) {
	if v, ok := c.GetPostForm("username"); ok {
		s.Perm.UserState().Confirm(v)
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "invalid credentials", BackLink: "/"})
		return
	}

	c.Redirect(http.StatusFound, "/admin")
}

func (s Server) ApiAdminUpdateStorage(c *gin.Context) {
	var (
		plugin string
		bucket string
		key    string
		value  string
	)
	if v, ok := c.GetPostForm("plugin"); ok {
		plugin = v
	} else {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}
	if v, ok := c.GetPostForm("bucket"); ok {
		bucket = v
	} else {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}
	if v, ok := c.GetPostForm("key"); ok {
		key = v
	} else {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}
	if v, ok := c.GetPostForm("value"); ok {
		value = v
	} else {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}

	var ps *PluginStorage
	ps, ok := s.Storage[plugin]
	if !ok {
		var err error
		ps, err = OpenPluginStorage(plugin)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
			return
		}
		s.Storage[plugin] = ps
	}

	err := ps.PutValue(bucket, key, value)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/admin"})
		return
	}

	c.Redirect(http.StatusFound, "/admin")
}

func (s Server) ApiAdminDeleteStorage(c *gin.Context) {
	var (
		plugin string
		bucket string
		key    string
	)
	if v, ok := c.GetPostForm("plugin"); ok {
		plugin = v
	} else {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}
	if v, ok := c.GetPostForm("bucket"); ok {
		bucket = v
	} else {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}
	if v, ok := c.GetPostForm("key"); ok {
		key = v
	} else {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}

	ps, ok := s.Storage[plugin]
	if !ok {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "cannot update storage", BackLink: "/admin"})
		return
	}

	err := ps.DeleteKey(bucket, key)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/admin"})
		return
	}

	c.Redirect(http.StatusFound, "/admin")
}

func (s Server) ApiAdminCSVStorage(c *gin.Context) {
	plugin, ok := c.GetPostForm("plugin")
	if !ok {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "plugin not found", BackLink: "/admin"})
		return
	}
	bucket, ok := c.GetPostForm("bucket")
	if !ok {
		c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: "bucket not found", BackLink: "/admin"})
		return
	}
	var resp string
	if ps, ok := s.Storage[plugin]; ok {
		var err error
		resp, err = ps.ToCSV(bucket)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", ErrorPage{Error: err.Error(), BackLink: "/admin"})
			return
		}
	}

	c.Data(http.StatusFound, "text/plain", []byte(resp))
	return
}
