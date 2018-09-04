package searchrefiner

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"github.com/hscells/groove/combinator"
)

func getSettings(s Server, c *gin.Context) Settings {
	username := s.UserState.Username(c.Request)

	var us Settings
	if v, ok := s.Settings[username]; ok {
		us = v
	} else {
		// Configure initial values for Settings struct.
	}

	return us
}

func (s Server) HandleSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "settings.html", getSettings(s, c))
	return
}

func (s Server) ApiSettingsRelevantSet(c *gin.Context) {
	sets := getSettings(s, c)

	var rel []int64
	err := c.BindJSON(&rel)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	d := make(combinator.Documents, len(rel))
	for i, r := range rel {
		d[i] = combinator.Document(r)
	}

	username := s.UserState.Username(c.Request)
	sets.Relevant = d

	s.Settings[username] = sets

	c.Status(http.StatusOK)
	return
}
