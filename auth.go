package searchrefiner

import (
	"github.com/gin-gonic/gin"
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
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if v, ok := c.GetPostForm("password"); ok {
		password = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "no password supplied", BackLink: "/account/login"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if s.UserState.CorrectPassword(username, password) {
		err := s.UserState.Login(c.Writer, username)
		if err != nil {
			c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: err.Error(), BackLink: "/account/login"})
			c.AbortWithError(http.StatusUnauthorized, err)
			return
		}
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
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if v, ok := c.GetPostForm("password"); ok {
		password = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "passwords do not match", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if v, ok := c.GetPostForm("password2"); ok {
		password2 = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "passwords do not match", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if password != password2 {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "passwords do not match", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if s.UserState.HasUser(username) {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "a user with that name already exists", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	isAdmin := false
	for _, u := range s.Config.Admins {
		if u == username {
			s.UserState.AddUser(username, password, username)
			s.UserState.SetAdminStatus(username)
			s.UserState.MarkConfirmed(username)
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		s.UserState.AddUser(username, password, username)
		s.UserState.AddUnconfirmed(username, "unconfirmed")
	}

	err := s.UserState.Login(c.Writer, username)
	if err != nil {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: err.Error(), BackLink: "/account/create"})
		c.AbortWithError(http.StatusUnauthorized, err)
		return
	}
	c.Redirect(http.StatusFound, "/")
	return
}

func (s Server) ApiAccountLogout(c *gin.Context) {
	username := s.UserState.Username(c.Request)
	if s.UserState.IsLoggedIn(username) {
		s.UserState.Logout(username)
		s.UserState.ClearCookie(c.Writer)
	}
	c.Redirect(http.StatusFound, "/account/login")
	return
}

func (s Server) ApiAccountUsername(c *gin.Context) {
	username := s.UserState.Username(c.Request)
	if !s.UserState.IsLoggedIn(username) {
		c.String(http.StatusOK, "anonymous")
		return
	}
	c.String(http.StatusOK, username)
	return
}

func (s Server) HandleAdmin(c *gin.Context) {
	u, err := s.UserState.AllUnconfirmedUsernames()
	if err != nil {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	type admin struct {
		Unconfirmed []string
	}

	c.HTML(http.StatusOK, "admin.html", admin{Unconfirmed: u})
}

func (s Server) ApiAdminConfirm(c *gin.Context) {
	if v, ok := c.GetPostForm("username"); ok {
		s.UserState.Confirm(v)
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", ErrorPage{Error: "invalid credentials", BackLink: "/"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Redirect(http.StatusFound, "/admin")
}
