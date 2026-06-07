package main

import (
	"os"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "f4kvs-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	store, err := f4kvs.Open(tempDir)
	if err != nil {
		panic(err)
	}
	chunkStore = &f4kvsChunkStore{store: store}
	defer func() {
		_ = chunkStore.Close()
	}()

	os.Exit(m.Run())
}

func TestStoreDocumentMetadata(t *testing.T) {
	tests := []struct {
		name string
		doc  model.LegalDocument
	}{
		{
			name: "store basic document",
			doc: model.LegalDocument{
				ID:      "test1",
				Title:   "Test Document",
				Content: "This is a test document",
			},
		},
		{
			name: "store document with special characters",
			doc: model.LegalDocument{
				ID:      "test2",
				Title:   "Test Document 你好",
				Content: "This is a test document with special characters: 你好, ñ, é",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storeDocumentMetadata(tt.doc)
			stored := loadStoredDocument(t, tt.doc.ID)

			if stored.ID != tt.doc.ID {
				t.Errorf("got ID %v, want %v", stored.ID, tt.doc.ID)
			}
			if stored.Title != tt.doc.Title {
				t.Errorf("got Title %v, want %v", stored.Title, tt.doc.Title)
			}
			if stored.Content != tt.doc.Content {
				t.Errorf("got Content %v, want %v", stored.Content, tt.doc.Content)
			}
		})
	}
}

func TestDatabaseInitialization(t *testing.T) {
	if err := chunkStoreMissingKey(t); err != nil {
		t.Errorf("chunk store initialization check failed: %v", err)
	}
}

func TestChunkStorePutGetScan(t *testing.T) {
	payload := []byte(`{"metadata":{"chunk_id":"chunk-1"},"text":"hello"}`)
	if err := chunkStore.Put("chunk:chunk-1", payload); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	got, err := chunkStore.Get("chunk:chunk-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}

	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(pairs) == 0 {
		t.Fatal("expected at least one scanned chunk")
	}
}
