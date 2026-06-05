package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseSearchQueriesExplicitRetrievalQ(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{Header: make(http.Header)}
	u, err := url.Parse("/search?q=From+excerpts+only:+summarize&rq=Marcus+Aurelius")
	if err != nil {
		t.Fatal(err)
	}
	c.Request.URL = u

	retrieval, generation, explicit := parseSearchQueries(c, "From excerpts only: summarize")
	if retrieval != "Marcus Aurelius" || !explicit {
		t.Fatalf("retrieval: got %q explicit=%v", retrieval, explicit)
	}
	if generation != "From excerpts only: summarize" {
		t.Fatalf("generation: got %q", generation)
	}
}

func TestParseSearchQueriesNoExplicitRQ(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{Header: make(http.Header)}
	c.Request.URL = &url.URL{Path: "/search"}

	retrieval, generation, explicit := parseSearchQueries(c, "obligations du president")
	if explicit || retrieval != "" {
		t.Fatalf("expected no explicit retrieval, got %q explicit=%v", retrieval, explicit)
	}
	if generation != "obligations du president" {
		t.Fatalf("generation: got %q", generation)
	}
}

func TestStripInstructionPrefixForRetrieval(t *testing.T) {
	long := "From the excerpts only: list 5 key ideas about Marcus Aurelius meditations virtue nature and duty according to stoic philosophy in the summaries provided"
	got := stripInstructionPrefixForRetrieval(long)
	if got == long {
		t.Fatalf("expected stripped query, got unchanged: %q", got)
	}
	if !strings.Contains(got, "Marcus") || !strings.Contains(got, "Aurelius") {
		t.Fatalf("stripped query should keep topic terms: %q", got)
	}
}
