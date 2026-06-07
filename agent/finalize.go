package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// finalize to be called once all documents have been ingested
func finalize(c *gin.Context) {
	if err := ragAgent.Finalize(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "finalize failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "finalized"})
}
