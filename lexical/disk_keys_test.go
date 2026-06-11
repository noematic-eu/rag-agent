package lexical

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestDiskNormalizeTermValidUTF8(t *testing.T) {
	long := strings.Repeat("é", 80) // 160 bytes, 80 runes
	norm := diskNormalizeTerm(long)
	if !utf8.ValidString(norm) {
		t.Fatal("normalized term is not valid UTF-8")
	}
	if len([]rune(norm)) != diskMaxTermLen {
		t.Fatalf("expected %d runes, got %d", diskMaxTermLen, len([]rune(norm)))
	}
	key := diskPostKey(norm)
	if !utf8.ValidString(key) {
		t.Fatalf("posting key is not valid UTF-8: %q", key)
	}
}

func TestDiskIndexLongUnicodeTerm(t *testing.T) {
	idx := newDiskIndex(newMapKV())
	chunk := model.Chunk{
		Metadata: model.ChunkMetadata{ChunkID: "unicode-chunk-0", DocID: "u", Corpus: "c1"},
		Text:     strings.Repeat("révolution ", 20),
	}
	if err := idx.IndexChunk(FieldsFromChunk(chunk)); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.Search("révolution", "c1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ChunkID != "unicode-chunk-0" {
		t.Fatalf("hits: %+v", hits)
	}
}
