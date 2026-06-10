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
	if !cfg.cragEnabled || cfg.agentEnabled {
		t.Fatalf("expected crag only: %+v", cfg)
	}

	cfg = parseSearchMode(newTestGinContext(map[string]string{"mode": "agent"}))
	if !cfg.cragEnabled || !cfg.agentEnabled {
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

func TestSearchModeLabel(t *testing.T) {
	if (searchModeConfig{agentEnabled: true}).modeLabel() != searchModeAgent {
		t.Fatal("expected agent label")
	}
	if (searchModeConfig{cragEnabled: true}).modeLabel() != searchModeCRAG {
		t.Fatal("expected crag label")
	}
}
