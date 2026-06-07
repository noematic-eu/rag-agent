package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestFuseMultiQueryRRF(t *testing.T) {
	lists := []map[string]int{
		{"a": 1, "b": 2},
		{"b": 1, "c": 2},
	}
	out := fuseMultiQueryRRF(lists)
	if len(out) != 3 {
		t.Fatalf("expected 3 fused hits, got %d", len(out))
	}
	if out[0].ID != "b" {
		t.Fatalf("expected b first after RRF fusion, got %+v", out)
	}
}

func TestDedupeQueries(t *testing.T) {
	out := dedupeQueries([]string{"a", "A", "b", "a"})
	if len(out) != 2 {
		t.Fatalf("expected 2 unique queries, got %v", out)
	}
}

func TestRankChunksMultiRejectsWhenPrimaryBelowMinScore(t *testing.T) {
	tempBleveDir := t.TempDir()
	tempChunkStoreDir := t.TempDir()
	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	llmConfig.EmbeddingsEnabled = false

	constitution := model.LegalDocument{
		ID:      "constitution-test",
		Title:   "constitution",
		Corpus:  "legal-demo",
		Content: "Titre premier - DE LA SOUVERAINETÉ\nARTICLE 1.\nSon principe est : gouvernement du peuple, par le peuple et pour le peuple.\n\nTitre II - LE PRÉSIDENT DE LA RÉPUBLIQUE\nARTICLE 5.\nLe Président de la République veille au respect de la Constitution. Il assure, par son arbitrage, le fonctionnement régulier des pouvoirs publics ainsi que la continuité de l'État.\n\nIl est le garant de l'indépendance nationale, de l'intégrité du territoire et du respect des traités.",
	}
	if _, err := indexDocument(constitution); err != nil {
		t.Fatalf("indexDocument: %v", err)
	}

	params := rankParams{
		topKBM25:     20,
		topKVector:   20,
		topKFinal:    8,
		minScore:     defaultMinScore,
		corpus:       "legal-demo",
		maxPerDoc:    6,
		fusionMode:   "weighted",
		fusionWeight: 0.6,
	}

	weakPrimary := "xyzqqq unrelated gibberish"
	strongSecondary := "article 5 président république"

	secondaryOnly := params
	secondaryOnly.retrievalText = strongSecondary
	secondaryOutcome, err := rankChunks(secondaryOnly)
	if err != nil {
		t.Fatalf("rankChunks secondary: %v", err)
	}
	if secondaryOutcome.noResults || len(secondaryOutcome.hits) == 0 {
		t.Fatalf("expected secondary query to produce hits, got noResults=%v", secondaryOutcome.noResults)
	}

	outcome, err := rankChunksMulti([]string{weakPrimary, strongSecondary}, params)
	if err != nil {
		t.Fatalf("rankChunksMulti: %v", err)
	}
	if !outcome.noResults {
		t.Fatalf("expected noResults when primary query is below min_score, got hits=%+v", outcome.hits)
	}
}
