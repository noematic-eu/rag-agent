package lexical

import (
	"fmt"
	"log"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
	"github.com/noematic-eu/f4kvs-lexical/lexindex"
)

const (
	F4KVSLexicalModeDisk = "disk"
	F4KVSLexicalModeRAM  = "ram"
)

type f4kvsDiskBackend struct {
	idx *lexindex.Index
}

func openF4KVSDisk(cfg Config) (Backend, error) {
	if cfg.KV == nil {
		return nil, fmt.Errorf("f4kvs disk lexical requires Config.KV")
	}
	idx := lexindex.New(cfg.KV)
	b := &f4kvsDiskBackend{idx: idx}
	if cfg.ScanChunks != nil {
		storeCount, err := countIndexableChunks(cfg.ScanChunks)
		if err != nil {
			return nil, err
		}
		indexed, _ := idx.ChunkCount()
		switch {
		case storeCount == 0:
			// empty store
		case !idx.HasMeta() && storeCount > 0:
			log.Printf("f4kvs lexical: no lex:meta yet (store=%d chunks); index at ingest or POST /finalize (incremental by default)", storeCount)
		case indexed < storeCount:
			log.Printf("f4kvs lexical: partial index (store=%d indexed=%d); retrieval_lex=auto uses scan fallback, or POST /finalize for catch-up", storeCount, indexed)
		}
	}
	return b, nil
}

func countIndexableChunks(scan func(yield func(model.Chunk) error) error) (int, error) {
	n := 0
	err := scan(func(chunk model.Chunk) error {
		f := FieldsFromChunk(chunk)
		if f.ChunkID != "" && f.Text != "" {
			n++
		}
		return nil
	})
	return n, err
}

func (b *f4kvsDiskBackend) Engine() string { return EngineF4KVS }

func (b *f4kvsDiskBackend) IndexChunk(chunk model.Chunk) error {
	return b.idx.IndexChunk(FieldsFromChunk(chunk))
}

func (b *f4kvsDiskBackend) DeleteChunk(chunkID string) error {
	return b.idx.DeleteChunk(chunkID)
}

func (b *f4kvsDiskBackend) Search(text, corpus string, k int) ([]Hit, error) {
	return b.idx.Search(text, corpus, k)
}

func (b *f4kvsDiskBackend) Close() error { return nil }

// RebuildF4KVSDisk rescans all chunks into the on-disk lexical index (wipes lex:* first).
func RebuildF4KVSDisk(b Backend, scan func(yield func(model.Chunk) error) error, chunksTotal int, onProgress RebuildProgressFunc) (RebuildStats, error) {
	disk, ok := b.(*f4kvsDiskBackend)
	if !ok {
		return RebuildStats{}, nil
	}
	return disk.idx.RebuildFromChunks(chunkScanFromModel(scan), chunksTotal, onProgress)
}

// RebuildF4KVSDiskIncremental indexes only chunks missing from lex:* (no lex wipe).
func RebuildF4KVSDiskIncremental(b Backend, scan func(yield func(model.Chunk) error) error, chunksTotal int, onProgress RebuildProgressFunc) (RebuildStats, error) {
	disk, ok := b.(*f4kvsDiskBackend)
	if !ok {
		return RebuildStats{}, nil
	}
	return disk.idx.RebuildIncrementalFromChunks(chunkScanFromModel(scan), chunksTotal, onProgress)
}

// F4KVSDiskChunkCount returns indexed chunks for the disk f4kvs backend.
func F4KVSDiskChunkCount(b Backend) int {
	disk, ok := b.(*f4kvsDiskBackend)
	if !ok {
		return 0
	}
	n, err := disk.idx.ChunkCount()
	if err != nil {
		return 0
	}
	return n
}

// F4KVSUsesDiskMode reports whether the backend persists lexical data on disk.
func F4KVSUsesDiskMode(b Backend) bool {
	_, ok := b.(*f4kvsDiskBackend)
	return ok
}

// F4KVSIsRebuilding reports whether the disk lexical index is mid-rebuild.
func F4KVSIsRebuilding(b Backend) bool {
	disk, ok := b.(*f4kvsDiskBackend)
	if !ok {
		return false
	}
	return disk.idx.IsRebuilding()
}

// ParseF4KVSLexicalMode normalizes disk vs ram mode.
func ParseF4KVSLexicalMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", F4KVSLexicalModeDisk, "on-disk":
		return F4KVSLexicalModeDisk
	case F4KVSLexicalModeRAM:
		return F4KVSLexicalModeRAM
	default:
		return F4KVSLexicalModeDisk
	}
}

// F4KVSIndexedChunkCount returns chunk count for either disk or RAM f4kvs backend.
func F4KVSIndexedChunkCount(b Backend) int {
	if n := F4KVSDiskChunkCount(b); n > 0 {
		return n
	}
	return F4KVSRamChunkCount(b)
}
