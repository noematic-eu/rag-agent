package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestArticleRefsFromQuery(t *testing.T) {
	refs := articleRefsFromQuery("article 16 pleins pouvoirs election presidentielle article 7")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %v", refs)
	}
	if refs[0] != "16" || refs[1] != "7" {
		t.Fatalf("unexpected refs: %v", refs)
	}
}

func TestFilterChunksByArticle(t *testing.T) {
	chunksByID := map[string]model.Chunk{
		"c16": {Metadata: model.ChunkMetadata{ChunkID: "c16", Article: "16", Title: "ARTICLE 16."}},
		"c7":  {Metadata: model.ChunkMetadata{ChunkID: "c7", Article: "7", Title: "ARTICLE 7."}},
	}
	sorted := []chunkScore{{ID: "c16", Score: 0.9}, {ID: "c7", Score: 0.8}}
	filtered := filterChunksByArticle(sorted, chunksByID, "7")
	if len(filtered) != 1 || filtered[0].ID != "c7" {
		t.Fatalf("unexpected filter result: %+v", filtered)
	}
}
