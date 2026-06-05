package main

import "github.com/gin-gonic/gin"

// finalize to be called once all documents have been ingested
func finalize(c *gin.Context) {
	_ = ragAgent.Finalize()
}
