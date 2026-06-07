package lexical

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestBleveIndexAndSearch(t *testing.T) {
	dir := t.TempDir()
	b, err := Open(Config{DataDir: dir, Engine: EngineBleve})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	chunk := model.Chunk{
		Metadata: model.ChunkMetadata{
			ChunkID:  "doc1-chunk-0",
			DocID:    "doc1",
			Title:    "Hybrid RAG",
			DocTitle: "rag-overview.md",
			Corpus:   "eval-public",
		},
		Text: "Retrieval augmented generation combines BM25 and vector search.",
	}
	if err := b.IndexChunk(chunk); err != nil {
		t.Fatal(err)
	}
	hits, err := b.Search("BM25 vector", "eval-public", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	if hits[0].ChunkID != "doc1-chunk-0" {
		t.Fatalf("got %s", hits[0].ChunkID)
	}
}
