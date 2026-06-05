package main

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestRetrieveReturnsHits(t *testing.T) {
	tempBleveDir, err := os.MkdirTemp("", "bleve-retrieve-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBleveDir)

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-retrieve-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempChunkStoreDir)

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()

	doc := model.LegalDocument{
		ID:      "retrieve-doc-1",
		Title:   "RAG Overview",
		Content: "# Hybrid RAG\n\nRetrieval augmented generation combines BM25 keyword search with vector embeddings for better recall.",
		Corpus:  "eval-public",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("index: %v", err)
	}

	c, rr, err := newGinContext("GET", "/retrieve?q=BM25+vector+embeddings&corpus=eval-public&top_k=5", nil)
	if err != nil {
		t.Fatal(err)
	}
	retrieveDocuments(c)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	var resp model.RetrieveResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s", resp.Status)
	}
	if len(resp.Hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if resp.Hits[0].ChunkID == "" || resp.Hits[0].DocID == "" {
		t.Fatalf("missing ids: %+v", resp.Hits[0])
	}
}

func TestRetrieveIncludeText(t *testing.T) {
	tempBleveDir, err := os.MkdirTemp("", "bleve-retrieve-text-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBleveDir)

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-retrieve-text-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempChunkStoreDir)

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()

	doc := model.LegalDocument{
		ID:      "retrieve-doc-text",
		Title:   "Hybrid RAG",
		Content: "# Hybrid RAG\n\nRetrieval augmented generation combines BM25 keyword search with vector embeddings.",
		Corpus:  "eval-public",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("index: %v", err)
	}

	c, rr, err := newGinContext("GET", "/retrieve?q=BM25+vector&corpus=eval-public&include_text=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	retrieveDocuments(c)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	var resp model.RetrieveResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Hits) == 0 {
		t.Fatal("expected hits")
	}
	if resp.Hits[0].Excerpt == "" {
		t.Fatalf("expected excerpt, got %+v", resp.Hits[0])
	}
}

func TestRetrieveEmptyQuery(t *testing.T) {
	c, rr, err := newGinContext("GET", "/retrieve", nil)
	if err != nil {
		t.Fatal(err)
	}
	retrieveDocuments(c)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
