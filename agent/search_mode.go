package main

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	searchModeDefault = ""
	searchModeCRAG    = "crag"
	searchModeAgent   = "agent"

	defaultCRAGMaxRounds = 2
	maxCRAGMaxRounds     = 2
	defaultAgentMaxIter  = 5
	maxAgentRetrievals   = 3
)

type searchModeConfig struct {
	cragEnabled   bool
	agentEnabled  bool
	cragMaxRounds int
}

func parseSearchMode(c *gin.Context) searchModeConfig {
	mode := strings.TrimSpace(strings.ToLower(c.Query("mode")))
	cragRaw := strings.TrimSpace(strings.ToLower(c.Query("crag")))

	cfg := searchModeConfig{cragMaxRounds: defaultCRAGMaxRounds}

	switch mode {
	case searchModeCRAG:
		cfg.cragEnabled = true
	case searchModeAgent:
		cfg.agentEnabled = true
		cfg.cragEnabled = true
	}

	switch cragRaw {
	case "1", "true", "yes", "on":
		cfg.cragEnabled = true
	case "0", "false", "no", "off":
		cfg.cragEnabled = false
	}

	if raw := strings.TrimSpace(c.Query("crag_max_rounds")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			cfg.cragMaxRounds = n
		}
	}
	if cfg.cragMaxRounds > maxCRAGMaxRounds {
		cfg.cragMaxRounds = maxCRAGMaxRounds
	}
	return cfg
}

func (c searchModeConfig) modeLabel() string {
	if c.agentEnabled {
		return searchModeAgent
	}
	if c.cragEnabled {
		return searchModeCRAG
	}
	return "default"
}
