package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func resetIndex(c *gin.Context) {
	if err := ragAgent.Reset(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reset failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "index reset"})
}
