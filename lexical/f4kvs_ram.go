package lexical

import (
	"log"
	"sort"
	"sync"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type f4kvsRamBackend struct {
	mu     sync.RWMutex
	global BM25Global
	chunks []BM25Chunk
}

func openF4KVSRam(cfg Config) (Backend, error) {
	b := &f4kvsRamBackend{
		global: BM25Global{DF: make(map[string]int)},
	}
	if cfg.ScanChunks != nil {
		if err := b.rebuild(cfg.ScanChunks); err != nil {
			return nil, err
		}
	}
	return b, nil
}

func (b *f4kvsRamBackend) Engine() string { return EngineF4KVS }

func (b *f4kvsRamBackend) rebuild(scan func(yield func(model.Chunk) error) error) error {
	start := len(b.chunks)
	_ = start
	g := BM25Global{DF: make(map[string]int)}
	var chunks []BM25Chunk
	n := 0
	err := scan(func(chunk model.Chunk) error {
		if chunk.Metadata.ChunkID == "" || chunk.Text == "" {
			return nil
		}
		registerChunkFields(&g, &chunks, FieldsFromChunk(chunk))
		n++
		return nil
	})
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.global = g
	b.chunks = chunks
	b.mu.Unlock()
	log.Printf("f4kvs lexical: rebuilt RAM BM25 index over %d chunks", n)
	return nil
}

func (b *f4kvsRamBackend) IndexChunk(chunk model.Chunk) error {
	f := FieldsFromChunk(chunk)
	if f.ChunkID == "" || f.Text == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeChunkLocked(f.ChunkID)
	registerChunkFields(&b.global, &b.chunks, f)
	return nil
}

func (b *f4kvsRamBackend) DeleteChunk(chunkID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeChunkLocked(chunkID)
	return nil
}

func (b *f4kvsRamBackend) removeChunkLocked(chunkID string) {
	if chunkID == "" {
		return
	}
	filtered := b.chunks[:0]
	for _, c := range b.chunks {
		if c.Fields.ChunkID == chunkID {
			b.global.unregisterChunk(c)
			continue
		}
		filtered = append(filtered, c)
	}
	b.chunks = filtered
}

func (b *f4kvsRamBackend) Search(text, corpus string, k int) ([]Hit, error) {
	query := Tokenize(text)
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.chunks) == 0 {
		return nil, nil
	}
	type scored struct {
		id    string
		score float64
	}
	scores := make([]scored, 0, len(b.chunks))
	for _, c := range b.chunks {
		if corpus != "" && c.Fields.Corpus != corpus {
			continue
		}
		s := ScoreChunkBM25(c, query, &b.global)
		if s > 0 {
			scores = append(scores, scored{id: c.Fields.ChunkID, score: s})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	if k > 0 && len(scores) > k {
		scores = scores[:k]
	}
	hits := make([]Hit, len(scores))
	for i, s := range scores {
		hits[i] = Hit{ChunkID: s.id, Score: s.score}
	}
	return hits, nil
}

func (b *f4kvsRamBackend) Close() error {
	b.mu.Lock()
	b.chunks = nil
	b.global = BM25Global{DF: make(map[string]int)}
	b.mu.Unlock()
	return nil
}

// ClearF4KVSRamIndex resets the process-wide RAM index (called on POST /reset).
func ClearF4KVSRamIndex(b Backend) {
	ram, ok := b.(*f4kvsRamBackend)
	if !ok {
		return
	}
	_ = ram.Close()
}
