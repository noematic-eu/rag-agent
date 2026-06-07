package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestTopKVectorHitsWithF4kvs(t *testing.T) {
	dir, err := os.MkdirTemp("", "f4kvs-vector-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	store, err := f4kvs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	chunkStore = &f4kvsChunkStore{store: store}
	t.Cleanup(func() { chunkStore = nil })

	chunks := []model.Chunk{
		{
			Metadata:  model.ChunkMetadata{ChunkID: "a", Corpus: "legal"},
			Text:      "alpha",
			Embedding: []float64{1, 0, 0},
		},
		{
			Metadata:  model.ChunkMetadata{ChunkID: "b", Corpus: "legal"},
			Text:      "beta",
			Embedding: []float64{0.9, 0.1, 0},
		},
		{
			Metadata:  model.ChunkMetadata{ChunkID: "c", Corpus: "other"},
			Text:      "gamma",
			Embedding: []float64{0, 1, 0},
		},
	}

	for _, chunk := range chunks {
		data, err := json.Marshal(chunk)
		if err != nil {
			t.Fatal(err)
		}
		if err := chunkStore.Put("chunk:"+chunk.Metadata.ChunkID, data); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := topKVectorHits([]float64{1, 0, 0}, 2, "legal")
	if err != nil {
		t.Fatalf("topKVectorHits failed: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].ChunkID != "a" {
		t.Fatalf("expected top hit a, got %s", hits[0].ChunkID)
	}
}
