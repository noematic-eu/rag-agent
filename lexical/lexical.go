package lexical

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
	"github.com/noematic-eu/f4kvs-lexical/lexindex"
)

const (
	EngineBleve   = "bleve"
	EngineTantivy = "tantivy"
	EngineF4KVS   = "f4kvs"
)

// Re-export field boosts from lexindex for Bleve/Tantivy backends.
const (
	BoostText     = lexindex.BoostText
	BoostTitle    = lexindex.BoostTitle
	BoostDocTitle = lexindex.BoostDocTitle
	BoostSection  = lexindex.BoostSection
	BoostArticle  = lexindex.BoostArticle
)

type (
	// Hit is one lexical retrieval result.
	Hit = lexindex.Hit
	// ChunkFields holds per-field text used for indexing and BM25.
	ChunkFields = lexindex.ChunkFields
	// KV is the minimal key-value API for the on-disk lexical index.
	KV = lexindex.KV
	// KVPair is a key/value entry from a prefix scan.
	KVPair = lexindex.KVPair
	// RebuildStats reports the outcome of a lexical index rebuild.
	RebuildStats = lexindex.RebuildStats
	// RebuildProgressFunc is called during rebuild.
	RebuildProgressFunc = lexindex.RebuildProgressFunc
)

// Backend indexes and searches chunks lexically.
type Backend interface {
	IndexChunk(chunk model.Chunk) error
	DeleteChunk(chunkID string) error
	Search(text, corpus string, k int) ([]Hit, error)
	Close() error
	Engine() string
}

// Config opens a lexical backend under DataDir.
type Config struct {
	DataDir          string
	Engine           string
	F4KVSLexicalMode string
	KV               KV
	ScanChunks       func(yield func(model.Chunk) error) error
}

func (c Config) BlevePath() string   { return filepath.Join(c.DataDir, "legal.bleve") }
func (c Config) TantivyPath() string { return filepath.Join(c.DataDir, "legal.tantivy") }

// ParseEngine normalizes and validates an engine name.
func ParseEngine(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	if e == "" {
		return EngineBleve, nil
	}
	switch e {
	case EngineBleve, EngineTantivy, EngineF4KVS:
		return e, nil
	default:
		return "", fmt.Errorf("unsupported lexical engine %q (want bleve, tantivy, or f4kvs)", raw)
	}
}

// Open creates the configured lexical backend.
func Open(cfg Config) (Backend, error) {
	engine, err := ParseEngine(cfg.Engine)
	if err != nil {
		return nil, err
	}
	cfg.Engine = engine
	switch engine {
	case EngineBleve:
		return openBleve(cfg)
	case EngineTantivy:
		return openTantivy(cfg)
	case EngineF4KVS:
		mode := ParseF4KVSLexicalMode(cfg.F4KVSLexicalMode)
		if mode == F4KVSLexicalModeRAM {
			return openF4KVSRam(cfg)
		}
		return openF4KVSDisk(cfg)
	default:
		return nil, fmt.Errorf("unsupported engine %q", engine)
	}
}

// FieldsFromChunk builds index fields from a chunk.
func FieldsFromChunk(chunk model.Chunk) ChunkFields {
	section := chunk.Metadata.Title
	if section == "" {
		section = chunk.Metadata.SectionPath
	}
	return ChunkFields{
		ChunkID:  chunk.Metadata.ChunkID,
		DocID:    chunk.Metadata.DocID,
		Corpus:   chunk.Metadata.Corpus,
		Text:     chunk.Text,
		Title:    chunk.Metadata.Title,
		DocTitle: chunk.Metadata.DocTitle,
		Section:  section,
		Article:  chunk.Metadata.Article,
	}
}

func chunkScanFromModel(scan func(yield func(model.Chunk) error) error) lexindex.ChunkScanFunc {
	if scan == nil {
		return nil
	}
	return func(yield func(ChunkFields) error) error {
		return scan(func(c model.Chunk) error {
			return yield(FieldsFromChunk(c))
		})
	}
}
