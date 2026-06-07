package main

import (
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestRunRetrievalPipelineFindsArticle5(t *testing.T) {
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

	query := "Quelles sont les obligations du président de la république envers le peuple."
	in := retrievalPipelineInput{
		generationQuery: query,
		params: rankParams{
			topKBM25:     20,
			topKVector:   20,
			topKFinal:    8,
			minScore:     defaultMinScore,
			corpus:       "legal-demo",
			maxPerDoc:    6,
			fusionMode:   "weighted",
			fusionWeight: 0.6,
		},
		rewriteEnabled: true,
	}
	outcome, queries, err := runRetrievalPipeline(in)
	if err != nil {
		t.Fatalf("runRetrievalPipeline: %v", err)
	}
	if outcome.noResults || len(outcome.hits) == 0 {
		t.Fatalf("expected hits, queries=%v", queries)
	}
	if len(queries) < 2 {
		t.Fatalf("expected expanded queries, got %v", queries)
	}

	foundArt5 := false
	for i, hit := range outcome.hits {
		if i >= 3 {
			break
		}
		if strings.EqualFold(hit.Article, "5") || strings.Contains(strings.ToUpper(hit.Section), "ARTICLE 5") {
			foundArt5 = true
			break
		}
	}
	if !foundArt5 {
		t.Fatalf("expected article 5 in top 3, got %+v queries=%v", outcome.hits, queries)
	}
}
