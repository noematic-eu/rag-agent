package lexical

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const (
	EngineBleve   = "bleve"
	EngineTantivy = "tantivy"
	EngineF4KVS   = "f4kvs"
)

// Field boost weights aligned with Bleve search.go.
const (
	BoostText     = 1.0
	BoostTitle    = 2.0
	BoostDocTitle = 2.5
	BoostSection  = 1.5
	BoostArticle  = 3.0
)

// Hit is one lexical retrieval result.
type Hit struct {
	ChunkID string
	Score   float64
}

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
	DataDir      string
	Engine       string
	ScanChunks   func(yield func(model.Chunk) error) error
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
		return openF4KVSRam(cfg)
	default:
		return nil, fmt.Errorf("unsupported engine %q", engine)
	}
}

// ChunkFields holds per-field text used for indexing and BM25.
type ChunkFields struct {
	ChunkID  string
	DocID    string
	Corpus   string
	Text     string
	Title    string
	DocTitle string
	Section  string
	Article  string
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
