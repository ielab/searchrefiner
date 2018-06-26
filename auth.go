package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func handleAccountLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "account_login.html", nil)
}

func handleAccountCreate(c *gin.Context) {
	c.HTML(http.StatusOK, "account_create.html", nil)
}

func (s server) apiAccountLogin(c *gin.Context) {
	var username, password string
	if v, ok := c.GetPostForm("username"); ok {
		username = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "no username supplied", BackLink: "/account/login"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if v, ok := c.GetPostForm("password"); ok {
		password = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "no password supplied", BackLink: "/account/login"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if s.UserState.CorrectPassword(username, password) {
		err := s.UserState.Login(c.Writer, username)
		if err != nil {
			c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: err.Error(), BackLink: "/account/login"})
			c.AbortWithError(http.StatusUnauthorized, err)
			return
		}
		c.Redirect(http.StatusFound, "/")
		return
	}
	c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "invalid login credentials", BackLink: "/account/login"})
	c.Status(http.StatusUnauthorized)
	return
}

func (s server) apiAccountCreate(c *gin.Context) {
	var username, password, password2 string
	if v, ok := c.GetPostForm("username"); ok {
		username = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "no username supplied", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if v, ok := c.GetPostForm("password"); ok {
		password = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "passwords do not match", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if v, ok := c.GetPostForm("password2"); ok {
		password2 = v
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "passwords do not match", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if password != password2 {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "passwords do not match", BackLink: "/account/create"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if s.UserState.HasUser(username) {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "a user with that name already exists", BackLink: "/account/create"})
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
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: err.Error(), BackLink: "/account/create"})
		c.AbortWithError(http.StatusUnauthorized, err)
		return
	}
	c.Redirect(http.StatusFound, "/")
	return
}

func (s server) apiAccountLogout(c *gin.Context) {
	username := s.UserState.Username(c.Request)
	if s.UserState.IsLoggedIn(username) {
		s.UserState.Logout(username)
		s.UserState.ClearCookie(c.Writer)
	}
	c.Redirect(http.StatusFound, "/account/login")
	return
}

func (s server) apiAccountUsername(c *gin.Context) {
	username := s.UserState.Username(c.Request)
	c.String(http.StatusOK, username)
	return
}

func (s server) handleAdmin(c *gin.Context) {
	u, err := s.UserState.AllUnconfirmedUsernames()
	if err != nil {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: err.Error(), BackLink: "/"})
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	type admin struct {
		Unconfirmed []string
	}

	c.HTML(http.StatusOK, "admin.html", admin{Unconfirmed: u})
}

func (s server) apiAdminConfirm(c *gin.Context) {
	if v, ok := c.GetPostForm("username"); ok {
		s.UserState.Confirm(v)
	} else {
		c.HTML(http.StatusUnauthorized, "error.html", errorPage{Error: "invalid credentials", BackLink: "/"})
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Redirect(http.StatusFound, "/admin")
}
