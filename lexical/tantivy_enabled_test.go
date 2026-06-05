//go:build tantivy

package lexical

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestTantivyIndexAndSearch(t *testing.T) {
	dir := t.TempDir()
	b, err := Open(Config{DataDir: dir, Engine: EngineTantivy})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	chunk := model.Chunk{
		Metadata: model.ChunkMetadata{
			ChunkID:  "t1-chunk-0",
			DocID:    "t1",
			Title:    "Chunking",
			DocTitle: "chunking.md",
			Corpus:   "eval-public",
		},
		Text: "Document chunking overlap token size",
	}
	if err := b.IndexChunk(chunk); err != nil {
		t.Fatal(err)
	}
	hits, err := b.Search("chunk overlap", "eval-public", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
}
