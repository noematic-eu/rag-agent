package lexical

import (
	"fmt"
	"testing"
	"time"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestDiskIndexSearch(t *testing.T) {
	idx := newDiskIndex(newMapKV())

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
	for _, c := range chunks {
		if err := idx.IndexChunk(FieldsFromChunk(c)); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := idx.Search("natural selection", "c1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ChunkID != "a-chunk-0" {
		t.Fatalf("hits: %+v", hits)
	}
}

func TestDiskIndexRebuild(t *testing.T) {
	idx := newDiskIndex(newMapKV())

	chunks := []model.Chunk{
		{
			Metadata: model.ChunkMetadata{ChunkID: "x-chunk-0", DocID: "x", Corpus: "c2"},
			Text:     "French Revolution causes estates general",
		},
	}
	_, err := idx.RebuildFromChunks(func(yield func(model.Chunk) error) error {
		for _, c := range chunks {
			if err := yield(c); err != nil {
				return err
			}
		}
		return nil
	}, len(chunks), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.HasMeta() {
		t.Fatal("expected meta after rebuild")
	}
	hits, err := idx.Search("French Revolution", "c2", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ChunkID != "x-chunk-0" {
		t.Fatalf("hits: %+v", hits)
	}
}

func TestDiskIndexDeleteChunk(t *testing.T) {
	idx := newDiskIndex(newMapKV())

	chunk := model.Chunk{
		Metadata: model.ChunkMetadata{ChunkID: "d-chunk-0", DocID: "d", Corpus: "c1"},
		Text:     "unique term zephyr winds",
	}
	if err := idx.IndexChunk(FieldsFromChunk(chunk)); err != nil {
		t.Fatal(err)
	}
	if err := idx.DeleteChunk("d-chunk-0"); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.Search("zephyr", "c1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits after delete, got %+v", hits)
	}
}

func TestDiskPostingListRoundtrip(t *testing.T) {
	entries := []diskPostingEntry{
		{ChunkID: "a", Corpus: "c1", TFText: 3, TFTitle: 1},
		{ChunkID: "b", Corpus: "c1", TFText: 1},
	}
	data := encodeDiskPostingList(entries)
	got, err := decodeDiskPostingList(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ChunkID != "a" || got[0].TFText != 3 {
		t.Fatalf("roundtrip: %+v", got)
	}
}

func TestDiskIndexHasMetaEmpty(t *testing.T) {
	idx := newDiskIndex(newMapKV())
	if idx.HasMeta() {
		t.Fatal("expected HasMeta false on empty index")
	}
	if err := idx.saveMeta(diskMeta{Version: 1, ChunkCount: 0}); err != nil {
		t.Fatal(err)
	}
	if idx.HasMeta() {
		t.Fatal("expected HasMeta false when chunk_count=0")
	}
}

func TestDiskIndexSearchDuringRebuild(t *testing.T) {
	idx := newDiskIndex(newMapKV())
	chunk := model.Chunk{
		Metadata: model.ChunkMetadata{ChunkID: "a-chunk-0", DocID: "a", Corpus: "c1"},
		Text:     "alpha beta gamma",
	}
	if err := idx.IndexChunk(FieldsFromChunk(chunk)); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		chunks := make([]model.Chunk, 0, 2000)
		for i := 0; i < 2000; i++ {
			chunks = append(chunks, model.Chunk{
				Metadata: model.ChunkMetadata{ChunkID: fmt.Sprintf("bulk-%d", i), DocID: "b", Corpus: "c1"},
				Text:     "lorem ipsum dolor sit amet repeated terms for rebuild",
			})
		}
		_, _ = idx.RebuildFromChunks(func(yield func(model.Chunk) error) error {
			for _, c := range chunks {
				if err := yield(c); err != nil {
					return err
				}
			}
			return nil
		}, len(chunks), nil)
	}()

	time.Sleep(10 * time.Millisecond)
	for i := 0; i < 20; i++ {
		if idx.IsRebuilding() {
			hits, err := idx.Search("alpha", "c1", 5)
			if err != nil {
				t.Fatalf("search during rebuild: %v", err)
			}
			if len(hits) != 0 {
				t.Fatalf("expected no hits during rebuild, got %+v", hits)
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	<-done
}

func TestDiskIndexRebuildProgress(t *testing.T) {
	idx := newDiskIndex(newMapKV())
	var calls [][2]int
	chunks := []model.Chunk{
		{
			Metadata: model.ChunkMetadata{ChunkID: "p-chunk-0", DocID: "p", Corpus: "c1"},
			Text:     "progress callback test chunk one",
		},
		{
			Metadata: model.ChunkMetadata{ChunkID: "p-chunk-1", DocID: "p", Corpus: "c1"},
			Text:     "progress callback test chunk two",
		},
	}
	stats, err := idx.RebuildFromChunks(func(yield func(model.Chunk) error) error {
		for _, c := range chunks {
			if err := yield(c); err != nil {
				return err
			}
		}
		return nil
	}, len(chunks), func(indexed, total int) {
		calls = append(calls, [2]int{indexed, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ChunksIndexed != 2 || stats.ChunksTotal != 2 {
		t.Fatalf("stats: %+v", stats)
	}
	if len(calls) == 0 || calls[len(calls)-1][0] != 2 {
		t.Fatalf("expected final progress callback, got %+v", calls)
	}
}

func TestF4KVSDiskBackendOpen(t *testing.T) {
	kv := newMapKV()
	chunks := []model.Chunk{
		{
			Metadata: model.ChunkMetadata{ChunkID: "a-chunk-0", DocID: "a", Corpus: "c1"},
			Text:     "Darwin natural selection species evolution",
		},
	}
	b, err := openF4KVSDisk(Config{
		KV: kv,
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
	hits, err := b.Search("natural selection", "c1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ChunkID != "a-chunk-0" {
		t.Fatalf("hits: %+v", hits)
	}
}
