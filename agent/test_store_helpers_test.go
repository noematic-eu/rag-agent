package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/lexical"
	"github.com/noematic-eu/ai-rag-agent/model"
)

func setupTestDatabases(t *testing.T, bleveDir, chunkStoreDir string) {
	t.Helper()

	store, err := f4kvs.Open(chunkStoreDir)
	if err != nil {
		t.Fatal(err)
	}
	chunkStore = &f4kvsChunkStore{store: store}

	storeCfg = agentConfig{
		DataDir:       bleveDir,
		LexicalEngine: lexical.EngineBleve,
	}
	var errOpen error
	lexicalBackend, errOpen = lexical.Open(lexical.Config{
		DataDir: bleveDir,
		Engine:  lexical.EngineBleve,
	})
	if errOpen != nil {
		t.Fatal(errOpen)
	}
	t.Cleanup(func() {
		_ = closeLexicalBackend()
	})

	documentTFIDFs = make([]DocumentTFIDF, 0)
	globalIDF = make(map[string]float64)
}

func firstStoredChunk(t *testing.T) model.Chunk {
	t.Helper()

	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		t.Fatalf("Failed to scan stored chunks: %v", err)
	}
	if len(pairs) == 0 {
		t.Fatal("no chunk found")
	}

	var chunk model.Chunk
	if err := json.Unmarshal(pairs[0].Value, &chunk); err != nil {
		t.Fatalf("Failed to unmarshal stored chunk: %v", err)
	}
	return chunk
}

func loadStoredDocument(t *testing.T, docID string) model.LegalDocument {
	t.Helper()

	data, err := chunkStore.Get("doc:" + docID)
	if err != nil {
		t.Fatalf("failed to retrieve document: %v", err)
	}

	var doc model.LegalDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to unmarshal document: %v", err)
	}
	return doc
}

func loadStoredChunkByID(t *testing.T, chunkID string) model.Chunk {
	t.Helper()

	data, err := chunkStore.Get("chunk:" + chunkID)
	if err != nil {
		t.Fatalf("failed to load stored chunk: %v", err)
	}

	var chunk model.Chunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		t.Fatalf("failed to unmarshal stored chunk: %v", err)
	}
	return chunk
}

func mustTempChunkStoreDir(t *testing.T, prefix string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func chunkStoreMissingKey(t *testing.T) error {
	t.Helper()

	_, err := chunkStore.Get("missing-key")
	if err == nil {
		return fmt.Errorf("expected missing key error")
	}
	if !errors.Is(err, f4kvs.ErrNotFound) {
		return fmt.Errorf("expected not found, got: %w", err)
	}
	return nil
}
