package lexical

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestF4KVSRamBM25(t *testing.T) {
	chunks := []model.Chunk{
		{
			Metadata: model.ChunkMetadata{ChunkID: "a-chunk-0", DocID: "a", Corpus: "c1"},
			Text:     "Darwin natural selection species evolution",
		},
		{
			Metadata: model.ChunkMetadata{ChunkID: "b-chunk-0", DocID: "b", Corpus: "c1"},
			Text:     "Newton gravity laws motion",
		},
	}
	b, err := Open(Config{
		Engine: EngineF4KVS,
		ScanChunks: func(yield func(model.Chunk) error) error {
			for _, c := range chunks {
				if err := yield(c); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	hits, err := b.Search("natural selection", "c1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ChunkID != "a-chunk-0" {
		t.Fatalf("hits: %+v", hits)
	}
}
