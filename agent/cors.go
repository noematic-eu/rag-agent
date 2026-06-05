package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	corsAllowedOrigins []string
	demoMode           bool
)

func initServerSecurity() {
	demoMode = parseBool(envOr("RAG_DEMO_MODE", "false"))
	raw := envOr("RAG_CORS_ORIGINS", "")
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			corsAllowedOrigins = append(corsAllowedOrigins, o)
		}
	}
	if demoMode {
		log.Printf("demo mode: mutating API endpoints disabled")
	}
	if len(corsAllowedOrigins) > 0 {
		log.Printf("CORS allowed origins: %s", strings.Join(corsAllowedOrigins, ", "))
	}
}

func corsMiddleware() gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(corsAllowedOrigins))
	for _, o := range corsAllowedOrigins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Accept")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func demoModeGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if demoMode {
			c.JSON(http.StatusForbidden, gin.H{"error": "demo mode: endpoint disabled"})
			c.Abort()
			return
		}
		c.Next()
	}
}
