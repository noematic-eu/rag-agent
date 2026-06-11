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
	searchModeAuto    = "auto"

	searchLevelAuto     = -1
	searchLevelRetrieve = 0

	defaultCRAGMaxRounds = 2
	maxCRAGMaxRounds     = 2
	defaultAgentMaxIter  = 5
	maxAgentRetrievals   = 3
)

type searchModeConfig struct {
	level         int
	autoEnabled   bool
	cragEnabled   bool
	agentEnabled  bool
	cragMaxRounds int
	escalation    escalationConfig
}

func parseSearchMode(c *gin.Context) searchModeConfig {
	mode := strings.TrimSpace(strings.ToLower(c.Query("mode")))
	cragRaw := strings.TrimSpace(strings.ToLower(c.Query("crag")))
	levelRaw := strings.TrimSpace(strings.ToLower(c.Query("level")))

	cfg := searchModeConfig{
		level:         searchLevelLinear,
		cragMaxRounds: defaultCRAGMaxRounds,
		escalation:    defaultEscalationConfig(),
	}

	switch levelRaw {
	case "auto":
		cfg.level = searchLevelAuto
		cfg.autoEnabled = true
	case "0":
		cfg.level = searchLevelRetrieve
	case "1", "":
		if levelRaw == "1" {
			cfg.level = searchLevelLinear
		}
	case "2":
		cfg.level = searchLevelCRAG
		cfg.cragEnabled = true
	case "3":
		cfg.level = searchLevelAgent
		cfg.cragEnabled = true
		cfg.agentEnabled = true
	default:
		if n, err := strconv.Atoi(levelRaw); err == nil {
			cfg.level = n
			if n >= searchLevelCRAG {
				cfg.cragEnabled = true
			}
			if n >= searchLevelAgent {
				cfg.agentEnabled = true
			}
		}
	}

	switch mode {
	case searchModeCRAG:
		cfg.level = searchLevelCRAG
		cfg.cragEnabled = true
	case searchModeAgent:
		cfg.level = searchLevelAgent
		cfg.agentEnabled = true
		cfg.cragEnabled = true
	case searchModeAuto:
		cfg.level = searchLevelAuto
		cfg.autoEnabled = true
		cfg.cragEnabled = false
		cfg.agentEnabled = false
	}

	switch cragRaw {
	case "1", "true", "yes", "on":
		if !cfg.autoEnabled {
			cfg.cragEnabled = true
			if cfg.level < searchLevelCRAG {
				cfg.level = searchLevelCRAG
			}
		}
	case "0", "false", "no", "off":
		if !cfg.autoEnabled && !cfg.agentEnabled {
			cfg.cragEnabled = false
			cfg.level = searchLevelLinear
		}
	}

	if raw := strings.TrimSpace(c.Query("crag_max_rounds")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			cfg.cragMaxRounds = n
		}
	}
	if cfg.cragMaxRounds > maxCRAGMaxRounds {
		cfg.cragMaxRounds = maxCRAGMaxRounds
	}

	cfg.escalation = parseEscalationConfig(c)
	return cfg
}

func parseEscalationConfig(c *gin.Context) escalationConfig {
	cfg := defaultEscalationConfig()
	if raw := strings.TrimSpace(c.Query("auto_min_linear_score")); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			cfg.minLinearScore = f
		}
	}
	if raw := strings.TrimSpace(c.Query("auto_crag_score")); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			cfg.cragScoreThreshold = f
		}
	}
	if raw := strings.TrimSpace(c.Query("auto_dominant_fraction")); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			cfg.dominantFraction = f
		}
	}
	return cfg
}

func (c searchModeConfig) modeLabel() string {
	if c.autoEnabled {
		return searchModeAuto
	}
	if c.agentEnabled {
		return searchModeAgent
	}
	if c.cragEnabled {
		return searchModeCRAG
	}
	return "default"
}

func (c searchModeConfig) requestedLevelLabel() string {
	if c.autoEnabled {
		return searchModeAuto
	}
	switch c.level {
	case searchLevelLinear:
		return "1"
	case searchLevelCRAG:
		return "2"
	case searchLevelAgent:
		return "3"
	default:
		return strconv.Itoa(c.level)
	}
}
