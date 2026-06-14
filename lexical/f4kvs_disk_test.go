package lexical

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestF4KVSDiskBackendOpen(t *testing.T) {
	kv := newDiskTestKV()
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
	if err := b.IndexChunk(chunks[0]); err != nil {
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

type diskTestKV struct {
	data map[string][]byte
}

func newDiskTestKV() *diskTestKV {
	return &diskTestKV{data: make(map[string][]byte)}
}

func (m *diskTestKV) Put(key string, value []byte) error {
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[key] = cp
	return nil
}

func (m *diskTestKV) Get(key string) ([]byte, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, errDiskTestNotFound
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (m *diskTestKV) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *diskTestKV) ScanPrefix(prefix string) ([]KVPair, error) {
	var pairs []KVPair
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			cp := make([]byte, len(v))
			copy(cp, v)
			pairs = append(pairs, KVPair{Key: k, Value: cp})
		}
	}
	return pairs, nil
}

type diskTestNotFound struct{}

func (diskTestNotFound) Error() string { return "not found" }

var errDiskTestNotFound = diskTestNotFound{}
