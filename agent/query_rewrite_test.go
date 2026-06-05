package main

import (
	"testing"
)

func TestIsAbstractLegalQueryPresidentObligations(t *testing.T) {
	q := "Quelles sont les obligations du président de la république envers le peuple."
	if !isAbstractLegalQuery(q) {
		t.Fatal("expected abstract legal query")
	}
}

func TestIsAbstractLegalQueryExplicitArticle(t *testing.T) {
	q := "article 16 pleins pouvoirs urgence"
	if isAbstractLegalQuery(q) {
		t.Fatal("expected non-abstract query with explicit article ref")
	}
}

func TestParseRewriteQueries(t *testing.T) {
	raw := "Requête 1 : article 5 president veille constitution\nRequête 2 : obligations president republique arbitrage"
	queries := parseRewriteQueries(raw)
	if len(queries) != 2 {
		t.Fatalf("expected 2 queries, got %v", queries)
	}
}

func TestBuildRetrievalQueriesExplicitRetrieval(t *testing.T) {
	queries := buildRetrievalQueries("generation q", "explicit rq", "legal-demo", true)
	if len(queries) != 1 || queries[0] != "explicit rq" {
		t.Fatalf("expected explicit retrieval only: %v", queries)
	}
}

func TestBuildRetrievalQueriesExpansion(t *testing.T) {
	queries := buildRetrievalQueries("obligations du president", "", "legal-demo", false)
	if len(queries) < 2 {
		t.Fatalf("expected expansion without LLM rewrite: %v", queries)
	}
}
