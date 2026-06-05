package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestRerankLegalArticlesBoostsElectionArticle(t *testing.T) {
	chunksByID := map[string]model.Chunk{
		"c16": {Metadata: model.ChunkMetadata{ChunkID: "c16", Article: "16", Title: "ARTICLE 16."}, Text: "mesures exceptionnelles"},
		"c11": {Metadata: model.ChunkMetadata{ChunkID: "c11", Article: "11", Title: "ARTICLE 11."}, Text: "referendum organisation pouvoirs publics"},
		"c7":  {Metadata: model.ChunkMetadata{ChunkID: "c7", Article: "7", Title: "ARTICLE 7."}, Text: "election presidentielle scrutin report"},
	}
	sorted := []chunkScore{
		{ID: "c11", Score: 0.9},
		{ID: "c16", Score: 0.85},
		{ID: "c7", Score: 0.8},
	}
	query := "pleins pouvoirs election presidentielle organisation scrutin"
	enabled := true
	out := rerankLegalArticles(sorted, chunksByID, query, "legal-demo", &enabled)
	if len(out) == 0 {
		t.Fatal("expected reranked hits")
	}
	idx7, idx16 := -1, -1
	for i, hit := range out {
		switch hit.ID {
		case "c7":
			idx7 = i
		case "c16":
			idx16 = i
		}
	}
	if idx7 < 0 || idx16 < 0 {
		t.Fatalf("missing expected articles in %+v", out)
	}
	if idx7 >= 2 {
		t.Fatalf("article 7 should rank in top 2 after rerank, got order: %+v", out)
	}
	if idx7 >= idx16 {
		t.Fatalf("article 7 should outrank article 16 for election query, got order: %+v", out)
	}
}

func TestLegalAffinityBonus(t *testing.T) {
	terms := normalizeLegalQueryTerms("pleins pouvoirs election presidentielle")
	if legalAffinityBonus(terms, "16") <= 0 {
		t.Fatal("expected affinity bonus for article 16")
	}
	if legalAffinityBonus(terms, "7") <= 0 {
		t.Fatal("expected affinity bonus for article 7")
	}
}

func TestRerankLegalArticlesBoostsPresidentRoleArticle(t *testing.T) {
	chunksByID := map[string]model.Chunk{
		"c1": {
			Metadata: model.ChunkMetadata{ChunkID: "c1", Article: "1", Title: "ARTICLE 1.", SectionPath: "Titre premier -> ARTICLE 1."},
			Text:     "gouvernement du peuple par le peuple pour le peuple",
		},
		"c5": {
			Metadata: model.ChunkMetadata{ChunkID: "c5", Article: "5", Title: "ARTICLE 5.", SectionPath: "Titre II - LE PRÉSIDENT DE LA RÉPUBLIQUE -> ARTICLE 5."},
			Text:     "Le Président veille au respect de la Constitution et assure la continuité de l'État",
		},
	}
	sorted := []chunkScore{
		{ID: "c1", Score: 0.9},
		{ID: "c5", Score: 0.7},
	}
	query := "obligations du president de la republique envers le peuple"
	enabled := true
	out := rerankLegalArticles(sorted, chunksByID, query, "legal-demo", &enabled)
	if len(out) == 0 || out[0].ID != "c5" {
		t.Fatalf("article 5 should rank first, got %+v", out)
	}
}
