package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestDecideEscalationSingleDocHighConfidence(t *testing.T) {
	outcome := frenchRevolutionFixtureOutcome()
	decision := decideEscalation(outcome, "Quelles étaient les causes de la Révolution française ?", 8, defaultEscalationConfig())
	if decision.Level != searchLevelLinear {
		t.Fatalf("expected level 1, got %d reason=%s", decision.Level, decision.Reason)
	}
	if decision.Reason != escalationReasonSingleDocHighConfidence {
		t.Fatalf("expected single_doc_high_confidence, got %s", decision.Reason)
	}
}

func TestDecideEscalationWeakScores(t *testing.T) {
	outcome := rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", DocID: "d1", Score: 0.4},
			{ChunkID: "c2", DocID: "d2", Score: 0.35},
		},
	}
	decision := decideEscalation(outcome, "What is chunk overlap?", 8, defaultEscalationConfig())
	if decision.Level != searchLevelCRAG {
		t.Fatalf("expected level 2, got %d reason=%s", decision.Level, decision.Reason)
	}
	if decision.Reason != escalationReasonWeakOrDispersed {
		t.Fatalf("expected weak_or_dispersed, got %s", decision.Reason)
	}
}

func TestDecideEscalationComparativeQuery(t *testing.T) {
	outcome := rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", DocID: "d1", Score: 0.7},
			{ChunkID: "c2", DocID: "d2", Score: 0.68},
		},
	}
	decision := decideEscalation(outcome, "Compare BM25 and vector retrieval", 8, defaultEscalationConfig())
	if decision.Level != searchLevelAgent {
		t.Fatalf("expected level 3, got %d reason=%s", decision.Level, decision.Reason)
	}
	if decision.Reason != escalationReasonMultihopQuery {
		t.Fatalf("expected multihop_query, got %s", decision.Reason)
	}
}

func TestDecideEscalationComparativeQuerySingleDocHighScore(t *testing.T) {
	outcome := rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", DocID: "d1", Score: 0.988},
		},
	}
	query := "Compare les causes de la Révolution française (1789) et celles de la Révolution américaine (1776)."
	decision := decideEscalation(outcome, query, 6, defaultEscalationConfig())
	if decision.Level != searchLevelAgent {
		t.Fatalf("expected level 3 for comparative query despite single-doc retrieve, got %d reason=%s", decision.Level, decision.Reason)
	}
	if decision.Reason != escalationReasonMultihopQuery {
		t.Fatalf("expected multihop_query, got %s", decision.Reason)
	}
}

func TestDecideEscalationDispersedDocs(t *testing.T) {
	outcome := rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", DocID: "d1", Score: 0.72},
			{ChunkID: "c2", DocID: "d2", Score: 0.71},
			{ChunkID: "c3", DocID: "d3", Score: 0.70},
		},
	}
	decision := decideEscalation(outcome, "pipeline stages", 8, defaultEscalationConfig())
	if decision.Level != searchLevelCRAG {
		t.Fatalf("expected level 2 for dispersed docs, got %d reason=%s", decision.Level, decision.Reason)
	}
}

func TestDecideEscalationDefaultLinear(t *testing.T) {
	outcome := rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", DocID: "d1", Score: 0.72},
			{ChunkID: "c2", DocID: "d1", Score: 0.70},
		},
	}
	decision := decideEscalation(outcome, "What is BM25?", 8, defaultEscalationConfig())
	if decision.Level != searchLevelLinear {
		t.Fatalf("expected level 1, got %d reason=%s", decision.Level, decision.Reason)
	}
	if decision.Reason != escalationReasonDefaultLinear {
		t.Fatalf("expected default_linear, got %s", decision.Reason)
	}
}

func TestShouldPostCRAGEscalateToAgent(t *testing.T) {
	decision := escalationDecision{
		Level: searchLevelCRAG,
		Signals: retrievalSignals{
			multihopQuery: true,
		},
	}
	trace := cragTrace{Sufficient: false}
	if !shouldPostCRAGEscalateToAgent(true, decision, trace) {
		t.Fatal("expected post-CRAG agent escalation")
	}
	if shouldPostCRAGEscalateToAgent(true, decision, cragTrace{Sufficient: true}) {
		t.Fatal("expected no escalation when CRAG sufficient")
	}
	if shouldPostCRAGEscalateToAgent(true, escalationDecision{Level: searchLevelCRAG}, trace) {
		t.Fatal("expected no escalation without multihop signal")
	}
}

func TestHasMultiAspectMarkers(t *testing.T) {
	if !hasMultiAspectMarkers("How does hybrid retrieval combine BM25 and vectors, and which metrics grade quality?") {
		t.Fatal("expected multi-aspect query")
	}
	if hasMultiAspectMarkers("What is BM25?") {
		t.Fatal("expected single-aspect query")
	}
}
