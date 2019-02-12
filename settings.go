package searchrefiner

import (
	"github.com/gin-gonic/gin"
	"github.com/hscells/groove/combinator"
	"net/http"
)

func GetSettings(s Server, c *gin.Context) Settings {
	username := s.Perm.UserState().Username(c.Request)

	var us Settings
	if v, ok := s.Settings[username]; ok {
		us = v
	} else {
		// Configure initial values for Settings struct.
	}

	return us
}

func (s Server) HandleSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "settings.html", GetSettings(s, c))
	return
}

func (s Server) ApiSettingsRelevantSet(c *gin.Context) {
	sets := GetSettings(s, c)

	var rel []int64
	err := c.BindJSON(&rel)
	if err != nil {
		c.String(http.StatusOK, err.Error())
		return
	}

	d := make(combinator.Documents, len(rel))
	for i, r := range rel {
		d[i] = combinator.Document(r)
	}

	username := s.Perm.UserState().Username(c.Request)
	sets.Relevant = d

	s.Settings[username] = sets

	c.Status(http.StatusOK)
	return
}
