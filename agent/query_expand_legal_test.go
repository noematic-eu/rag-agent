package main

import (
	"strings"
	"testing"
)

func TestExpandLegalQueryPresidentObligations(t *testing.T) {
	queries := expandLegalQuery("Quelles sont les obligations du président de la république envers le peuple.")
	if len(queries) < 2 {
		t.Fatalf("expected expansion, got %v", queries)
	}
	expanded := strings.ToLower(queries[1])
	for _, want := range []string{"article 5", "veille", "constitution", "arbitrage"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded query missing %q: %s", want, expanded)
		}
	}
}

func TestExpandLegalQueryPresidentElection(t *testing.T) {
	queries := expandLegalQuery("report election presidentielle scrutin")
	if len(queries) < 2 {
		t.Fatalf("expected expansion, got %v", queries)
	}
	expanded := strings.ToLower(queries[1])
	if !strings.Contains(expanded, "article 7") {
		t.Fatalf("expected article 7 expansion: %s", expanded)
	}
}

func TestExpandLegalQueryDedupes(t *testing.T) {
	queries := expandLegalQuery("article 5 president obligations")
	if len(queries) != len(uniqueStrings(queries)) {
		t.Fatalf("expected unique queries: %v", queries)
	}
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
