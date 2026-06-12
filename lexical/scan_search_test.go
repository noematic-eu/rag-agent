package lexical

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestSearchBM25ScanCorpusFilter(t *testing.T) {
	chunks := []model.Chunk{
		{
			Metadata: model.ChunkMetadata{ChunkID: "a-0", DocID: "a", Corpus: "c1"},
			Text:     "French Revolution causes estates general",
		},
		{
			Metadata: model.ChunkMetadata{ChunkID: "b-0", DocID: "b", Corpus: "c2"},
			Text:     "Newton gravity laws motion physics",
		},
	}
	scan := func(yield func(model.Chunk) error) error {
		for _, c := range chunks {
			if err := yield(c); err != nil {
				return err
			}
		}
		return nil
	}

	hits, err := SearchBM25Scan(scan, "French Revolution", "c1", 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ChunkID != "a-0" {
		t.Fatalf("hits: %+v", hits)
	}

	hits, err = SearchBM25Scan(scan, "French Revolution", "c2", 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits for wrong corpus, got %+v", hits)
	}
}
