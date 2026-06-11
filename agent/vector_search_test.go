package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestTopKVectorHitsWithEmbedStore(t *testing.T) {
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
	llmConfig.EmbeddingsEnabled = true
	t.Cleanup(func() {
		chunkStore = nil
		llmConfig.EmbeddingsEnabled = false
	})

	chunks := []model.Chunk{
		{
			Metadata:  model.ChunkMetadata{ChunkID: "a", DocID: "d1", Corpus: "legal"},
			Text:      "alpha",
			Embedding: []float64{1, 0, 0},
		},
		{
			Metadata:  model.ChunkMetadata{ChunkID: "b", DocID: "d1", Corpus: "legal"},
			Text:      "beta",
			Embedding: []float64{0.9, 0.1, 0},
		},
		{
			Metadata:  model.ChunkMetadata{ChunkID: "c", DocID: "d2", Corpus: "other"},
			Text:      "gamma",
			Embedding: []float64{0, 1, 0},
		},
	}

	for _, chunk := range chunks {
		chunkCopy := chunk
		chunkCopy.Embedding = nil
		data, err := json.Marshal(chunkCopy)
		if err != nil {
			t.Fatal(err)
		}
		if err := chunkStore.Put("chunk:"+chunk.Metadata.ChunkID, data); err != nil {
			t.Fatal(err)
		}
		if err := storeEmbedRecord(chunk); err != nil {
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

func TestMaybeMigrateEmbedsFromChunks(t *testing.T) {
	dir, err := os.MkdirTemp("", "f4kvs-embed-migrate-*")
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
	llmConfig.EmbeddingsEnabled = true
	t.Cleanup(func() {
		chunkStore = nil
		llmConfig.EmbeddingsEnabled = false
	})

	legacy := model.Chunk{
		Metadata:  model.ChunkMetadata{ChunkID: "legacy-1", DocID: "d1", Corpus: "c1"},
		Text:      "hello world",
		Embedding: []float64{1, 0},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := chunkStore.Put("chunk:legacy-1", data); err != nil {
		t.Fatal(err)
	}

	if err := maybeMigrateEmbedsFromChunks(); err != nil {
		t.Fatal(err)
	}
	meta, ok, err := loadEmbedMeta()
	if err != nil || !ok || meta.Count != 1 {
		t.Fatalf("meta after migration: ok=%v count=%d err=%v", ok, meta.Count, err)
	}
	hits, err := topKVectorHits([]float64{1, 0}, 1, "c1")
	if err != nil || len(hits) != 1 || hits[0].ChunkID != "legacy-1" {
		t.Fatalf("hits: %+v err=%v", hits, err)
	}
}
