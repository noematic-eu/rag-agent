package main

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestGinContext(query map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{Header: make(http.Header)}
	values := url.Values{}
	for k, v := range query {
		values.Set(k, v)
	}
	c.Request.URL = &url.URL{Path: "/search", RawQuery: values.Encode()}
	return c
}

func TestParseSearchMode(t *testing.T) {
	cfg := parseSearchMode(newTestGinContext(map[string]string{"mode": "crag"}))
	if !cfg.cragEnabled || cfg.agentEnabled || cfg.level != searchLevelCRAG {
		t.Fatalf("expected crag only: %+v", cfg)
	}

	cfg = parseSearchMode(newTestGinContext(map[string]string{"mode": "agent"}))
	if !cfg.cragEnabled || !cfg.agentEnabled || cfg.level != searchLevelAgent {
		t.Fatalf("expected agent with crag: %+v", cfg)
	}

	cfg = parseSearchMode(newTestGinContext(map[string]string{"crag": "1"}))
	if !cfg.cragEnabled {
		t.Fatalf("expected crag=1: %+v", cfg)
	}

	cfg = parseSearchMode(newTestGinContext(map[string]string{"crag_max_rounds": "5"}))
	if cfg.cragMaxRounds != maxCRAGMaxRounds {
		t.Fatalf("expected capped rounds %d, got %d", maxCRAGMaxRounds, cfg.cragMaxRounds)
	}
}

func TestParseSearchModeLevelAuto(t *testing.T) {
	cfg := parseSearchMode(newTestGinContext(map[string]string{"level": "auto"}))
	if !cfg.autoEnabled || cfg.cragEnabled || cfg.agentEnabled {
		t.Fatalf("expected auto without preset agentic flags: %+v", cfg)
	}
	if cfg.level != searchLevelAuto {
		t.Fatalf("expected auto level, got %d", cfg.level)
	}
}

func TestParseSearchModeExplicitLevels(t *testing.T) {
	cfg := parseSearchMode(newTestGinContext(map[string]string{"level": "2"}))
	if !cfg.cragEnabled || cfg.agentEnabled || cfg.level != searchLevelCRAG {
		t.Fatalf("expected level 2: %+v", cfg)
	}

	cfg = parseSearchMode(newTestGinContext(map[string]string{"level": "3"}))
	if !cfg.cragEnabled || !cfg.agentEnabled || cfg.level != searchLevelAgent {
		t.Fatalf("expected level 3: %+v", cfg)
	}
}

func TestParseSearchModeEscalationOverrides(t *testing.T) {
	cfg := parseSearchMode(newTestGinContext(map[string]string{
		"level":                  "auto",
		"auto_min_linear_score":  "0.9",
		"auto_crag_score":        "0.6",
		"auto_dominant_fraction": "0.75",
	}))
	if cfg.escalation.minLinearScore != 0.9 {
		t.Fatalf("expected min linear score override, got %v", cfg.escalation.minLinearScore)
	}
	if cfg.escalation.cragScoreThreshold != 0.6 {
		t.Fatalf("expected crag score override, got %v", cfg.escalation.cragScoreThreshold)
	}
	if cfg.escalation.dominantFraction != 0.75 {
		t.Fatalf("expected dominant fraction override, got %v", cfg.escalation.dominantFraction)
	}
}

func TestSearchModeLabel(t *testing.T) {
	if (searchModeConfig{agentEnabled: true}).modeLabel() != searchModeAgent {
		t.Fatal("expected agent label")
	}
	if (searchModeConfig{cragEnabled: true}).modeLabel() != searchModeCRAG {
		t.Fatal("expected crag label")
	}
	if (searchModeConfig{autoEnabled: true}).modeLabel() != searchModeAuto {
		t.Fatal("expected auto label")
	}
}
