package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestParseAgentActionSearch(t *testing.T) {
	raw := "ACTION: search_kb\nQUERY: article 7 election\nCORPUS: legal-demo\n"
	action := parseAgentAction(raw)
	if action.Name != "search_kb" || action.Query != "article 7 election" || action.Corpus != "legal-demo" {
		t.Fatalf("unexpected action: %+v", action)
	}
}

func TestParseAgentActionFinish(t *testing.T) {
	raw := "ACTION: finish\nREASON: both articles covered\n"
	action := parseAgentAction(raw)
	if action.Name != "finish" || action.Reason == "" {
		t.Fatalf("unexpected action: %+v", action)
	}
}

func TestAgentToolContextDedupesChunks(t *testing.T) {
	ctx := newAgentToolContext([]model.LegalDocument{
		{ID: "doc1::c1", Title: "A", Content: "one"},
		{ID: "doc1::c1", Title: "A", Content: "one dup"},
		{ID: "doc1::c2", Title: "B", Content: "two"},
	})
	if len(ctx.collectedDocs) != 2 {
		t.Fatalf("expected 2 unique docs, got %d", len(ctx.collectedDocs))
	}
}
