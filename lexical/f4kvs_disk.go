package lexical

import (
	"fmt"
	"log"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const (
	F4KVSLexicalModeDisk = "disk"
	F4KVSLexicalModeRAM  = "ram"
)

type f4kvsDiskBackend struct {
	idx *diskIndex
}

func openF4KVSDisk(cfg Config) (Backend, error) {
	if cfg.KV == nil {
		return nil, fmt.Errorf("f4kvs disk lexical requires Config.KV")
	}
	idx := newDiskIndex(cfg.KV)
	b := &f4kvsDiskBackend{idx: idx}
	if cfg.ScanChunks != nil {
		storeCount, err := countIndexableChunks(cfg.ScanChunks)
		if err != nil {
			return nil, err
		}
		indexed, _ := idx.ChunkCount()
		needsRebuild := !idx.HasMeta() || (storeCount >= 100 && indexed < storeCount/2)
		if needsRebuild {
			log.Printf("f4kvs lexical: rebuilding disk index (store=%d indexed=%d)", storeCount, indexed)
			if _, err := idx.RebuildFromChunks(cfg.ScanChunks, storeCount, nil); err != nil {
				return nil, err
			}
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

// RebuildF4KVSDisk rescans chunks into the on-disk lexical index.
func RebuildF4KVSDisk(b Backend, scan func(yield func(model.Chunk) error) error, chunksTotal int, onProgress RebuildProgressFunc) (RebuildStats, error) {
	disk, ok := b.(*f4kvsDiskBackend)
	if !ok {
		return RebuildStats{}, nil
	}
	return disk.idx.RebuildFromChunks(scan, chunksTotal, onProgress)
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
