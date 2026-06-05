package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestDedupeByDocIDUsesChunkDocID(t *testing.T) {
	chunksByID := map[string]model.Chunk{
		"doc-a-chunk-0": {
			Metadata: model.ChunkMetadata{DocID: "doc-a", ChunkID: "doc-a-chunk-0"},
		},
		"doc-a-chunk-1": {
			Metadata: model.ChunkMetadata{DocID: "doc-a", ChunkID: "doc-a-chunk-1"},
		},
		"doc-a-chunk-2": {
			Metadata: model.ChunkMetadata{DocID: "doc-a", ChunkID: "doc-a-chunk-2"},
		},
		"doc-b-chunk-0": {
			Metadata: model.ChunkMetadata{DocID: "doc-b", ChunkID: "doc-b-chunk-0"},
		},
	}

	hits := []chunkScore{
		{ID: "doc-a-chunk-0", Score: 1.0},
		{ID: "doc-a-chunk-1", Score: 0.9},
		{ID: "doc-a-chunk-2", Score: 0.8},
		{ID: "doc-b-chunk-0", Score: 0.7},
	}

	deduped := dedupeByDocID(hits, chunksByID, 2)
	if len(deduped) != 3 {
		t.Fatalf("expected 3 hits after dedupe (2 from doc-a, 1 from doc-b), got %d", len(deduped))
	}
	if deduped[0].ID != "doc-a-chunk-0" || deduped[1].ID != "doc-a-chunk-1" {
		t.Fatalf("unexpected doc-a chunks: %+v", deduped[:2])
	}
	if deduped[2].ID != "doc-b-chunk-0" {
		t.Fatalf("expected doc-b chunk, got %s", deduped[2].ID)
	}
}

func TestDedupeByDocIDWithoutHydrationIsBroken(t *testing.T) {
	hits := []chunkScore{
		{ID: "doc-a-chunk-0", Score: 1.0},
		{ID: "doc-a-chunk-1", Score: 0.9},
	}
	deduped := dedupeByDocID(hits, map[string]model.Chunk{}, 2)
	if len(deduped) != 2 {
		t.Fatalf("empty chunksByID does not dedupe by document (documents the bug); got %d hits", len(deduped))
	}
}
