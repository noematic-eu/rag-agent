package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func setupEmbeddingTestStores(t *testing.T) {
	t.Helper()

	tempBleveDir := mustTempChunkStoreDir(t, "bleve-embedding-test-*")
	tempChunkStoreDir := mustTempChunkStoreDir(t, "f4kvs-embedding-test-*")

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	t.Cleanup(func() {
		_ = closeLexicalBackend()
		if chunkStore != nil {
			_ = chunkStore.Close()
			chunkStore = nil
		}
	})

}

func TestEmbedTextBatchCountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2]]}`))
	}))
	defer server.Close()

	previousConfig := llmConfig
	llmConfig = LLMConfig{
		Provider:          ProviderOllama,
		BaseURL:           server.URL,
		EmbeddingModel:    "nomic-embed-text",
		EmbeddingsEnabled: true,
	}
	t.Cleanup(func() { llmConfig = previousConfig })

	_, err := EmbedTextBatch([]string{"one", "two"})
	if err == nil {
		t.Fatal("expected error for embedding count mismatch")
	}
}

func TestEmbedTextBatchHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream error", http.StatusInternalServerError)
	}))
	defer server.Close()

	previousConfig := llmConfig
	llmConfig = LLMConfig{
		Provider:          ProviderOllama,
		BaseURL:           server.URL,
		EmbeddingModel:    "nomic-embed-text",
		EmbeddingsEnabled: true,
	}
	t.Cleanup(func() { llmConfig = previousConfig })

	_, err := EmbedTextBatch([]string{"test"})
	if err == nil {
		t.Fatal("expected error for non-2xx HTTP response")
	}
}

func TestIndexDocumentStoresChunkEmbedding(t *testing.T) {
	setupEmbeddingTestStores(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.11,0.22,0.33]]}`))
	}))
	defer server.Close()

	previousConfig := llmConfig
	llmConfig = LLMConfig{
		Provider:          ProviderOllama,
		BaseURL:           server.URL,
		EmbeddingModel:    "nomic-embed-text",
		EmbeddingsEnabled: true,
	}
	t.Cleanup(func() { llmConfig = previousConfig })

	doc := model.LegalDocument{
		ID:      "embed-doc",
		Title:   "Embedding Test",
		Content: "# Intro\n\nThis is enough content to build at least one chunk for storage in f4kvs with embeddings attached after batch processing during document ingestion in the test harness.",
	}

	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("indexDocument returned error: %v", err)
	}

	stored := loadStoredChunkByID(t, "embed-doc-chunk-0")
	if len(stored.Embedding) != 0 {
		t.Fatal("expected chunk store to omit embedding (stored in embed:*)")
	}
	data, err := chunkStore.Get(embedKey("embed-doc-chunk-0"))
	if err != nil {
		t.Fatalf("expected embed record: %v", err)
	}
	var rec EmbedRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("decode embed record: %v", err)
	}
	if len(rec.Embedding) == 0 {
		t.Fatal("expected non-empty embedding in embed store")
	}
}
