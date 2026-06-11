package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestApplyEscalationDecision(t *testing.T) {
	mode := searchModeConfig{autoEnabled: true, level: searchLevelAuto}
	decision := escalationDecision{Level: searchLevelCRAG, Reason: escalationReasonWeakOrDispersed}
	resolved := applyEscalationDecision(mode, decision)
	if !resolved.cragEnabled || resolved.agentEnabled {
		t.Fatalf("expected CRAG only: %+v", resolved)
	}
	if resolved.level != searchLevelCRAG {
		t.Fatalf("expected level 2, got %d", resolved.level)
	}
}

func TestRunSearchWithAgenticModesUsesInitialOutcome(t *testing.T) {
	initial := frenchRevolutionFixtureOutcome()
	opts := agenticSearchOptions{initialOutcome: &initial}
	if opts.initialOutcome == nil || len(opts.initialOutcome.hits) != 3 {
		t.Fatal("expected precomputed initial outcome with 3 hits")
	}
	if opts.initialOutcome.hits[0].Score != 0.97 {
		t.Fatalf("unexpected top score: %v", opts.initialOutcome.hits[0].Score)
	}
}

func TestEscalationPayload(t *testing.T) {
	decision := decideEscalation(frenchRevolutionFixtureOutcome(), "causes", 8, defaultEscalationConfig())
	payload := escalationPayload(decision)
	if payload["resolved_level"] != searchLevelLinear {
		t.Fatalf("expected resolved level 1, got %v", payload["resolved_level"])
	}
	if payload["reason"] != escalationReasonSingleDocHighConfidence {
		t.Fatalf("unexpected reason: %v", payload["reason"])
	}
}

func TestMultihopEscalationToAgent(t *testing.T) {
	outcome := rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", DocID: "d1", Score: 0.72},
			{ChunkID: "c2", DocID: "d2", Score: 0.71},
		},
	}
	decision := decideEscalation(outcome, "Compare BM25 and vector retrieval", 8, defaultEscalationConfig())
	resolved := applyEscalationDecision(searchModeConfig{autoEnabled: true}, decision)
	if !resolved.agentEnabled {
		t.Fatal("expected agent enabled for comparative query")
	}
}
