package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"github.com/hscells/groove/combinator"
)

type settings struct {
	Relevant combinator.Documents
}

func getSettings(s server, c *gin.Context) settings {
	username := s.UserState.Username(c.Request)

	var us settings
	if v, ok := s.Settings[username]; ok {
		us = v
	} else {
		// Configure initial values for settings struct.
	}

	return us
}

func (s server) handleSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "settings.html", getSettings(s, c))
	return
}

func (s server) apiSettingsRelevantSet(c *gin.Context) {
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
