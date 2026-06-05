package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
)

const (
	indexManifestKey = "meta:index_manifest"
	ingestStatsKey   = "meta:ingest_stats"
)

type indexManifest struct {
	SchemaVersion     int       `json:"schema_version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	EmbeddingModel    string    `json:"embedding_model"`
	EmbeddingsEnabled bool      `json:"embeddings_enabled"`
	LexicalEngine     string    `json:"lexical_engine,omitempty"`
	ChunkingVersion   string    `json:"chunking_version"`
	PipelineVersion   string    `json:"pipeline_version"`
}

type corpusIngestStats struct {
	DocumentsTotal     int       `json:"documents_total"`
	ChunksTotal        int       `json:"chunks_total"`
	EmbeddedChunks     int       `json:"embedded_chunks"`
	EmbeddingFailures  int       `json:"embedding_failures"`
	LastIngestAt       time.Time `json:"last_ingest_at,omitempty"`
	LastIngestDocument string    `json:"last_ingest_document,omitempty"`
}

type ingestStats struct {
	DocumentsTotal    int                           `json:"documents_total"`
	ChunksTotal       int                           `json:"chunks_total"`
	EmbeddedChunks    int                           `json:"embedded_chunks"`
	EmbeddingFailures int                           `json:"embedding_failures"`
	LastIngestAt      time.Time                     `json:"last_ingest_at,omitempty"`
	ByCorpus          map[string]*corpusIngestStats `json:"by_corpus,omitempty"`
}

type statsSnapshot struct {
	Manifest      indexManifest `json:"manifest"`
	Ingest        ingestStats   `json:"ingest"`
	Compatibility struct {
		Compatible bool     `json:"compatible"`
		Warnings   []string `json:"warnings,omitempty"`
	} `json:"compatibility"`
}

var statsState = struct {
	mu       sync.RWMutex
	manifest indexManifest
	ingest   ingestStats
}{}

func chunkingVersion() string {
	cfg := DefaultChunkConfig()
	return fmt.Sprintf("max=%d;overlap=%d;min=%d;sep=%q", cfg.MaxTokens, cfg.OverlapTokens, cfg.MinChunkSize, cfg.Separator)
}

func defaultManifest() indexManifest {
	now := time.Now().UTC()
	lexEngine := "bleve"
	if storeCfg.LexicalEngine != "" {
		lexEngine = storeCfg.LexicalEngine
	}
	return indexManifest{
		SchemaVersion:     1,
		CreatedAt:         now,
		UpdatedAt:         now,
		EmbeddingModel:    llmConfig.EmbeddingModel,
		EmbeddingsEnabled: llmConfig.EmbeddingsEnabled,
		LexicalEngine:     lexEngine,
		ChunkingVersion:   chunkingVersion(),
		PipelineVersion:   "rag-agent-v1",
	}
}

func defaultIngestStats() ingestStats {
	return ingestStats{
		ByCorpus: make(map[string]*corpusIngestStats),
	}
}

func loadStatsFromStore() error {
	statsState.mu.Lock()
	defer statsState.mu.Unlock()

	manifest, err := readManifestFromStore()
	if err != nil {
		return err
	}
	stats, err := readIngestStatsFromStore()
	if err != nil {
		return err
	}

	statsState.manifest = manifest
	statsState.ingest = stats
	return nil
}

func resetStatsState() {
	statsState.mu.Lock()
	defer statsState.mu.Unlock()
	statsState.manifest = defaultManifest()
	statsState.ingest = defaultIngestStats()
}

func ensureManifestInitialized() error {
	statsState.mu.Lock()
	defer statsState.mu.Unlock()

	if statsState.manifest.SchemaVersion != 0 {
		return nil
	}
	statsState.manifest = defaultManifest()
	return writeManifestToStore(statsState.manifest)
}

func recordDocumentDelete(docID, corpus string, chunks int) {
	statsState.mu.Lock()
	defer statsState.mu.Unlock()

	if statsState.ingest.ByCorpus == nil {
		statsState.ingest.ByCorpus = make(map[string]*corpusIngestStats)
	}

	if statsState.ingest.ChunksTotal >= chunks {
		statsState.ingest.ChunksTotal -= chunks
	}
	if statsState.ingest.DocumentsTotal > 0 {
		statsState.ingest.DocumentsTotal--
	}

	corpusKey := corpus
	if corpusKey == "" {
		corpusKey = "_default"
	}
	if entry := statsState.ingest.ByCorpus[corpusKey]; entry != nil {
		if entry.ChunksTotal >= chunks {
			entry.ChunksTotal -= chunks
		}
		if entry.DocumentsTotal > 0 {
			entry.DocumentsTotal--
		}
	}

	statsState.manifest.UpdatedAt = time.Now().UTC()
	_ = writeManifestToStore(statsState.manifest)
	_ = writeIngestStatsToStore(statsState.ingest)
	_ = docID // doc id kept for future per-doc stats if needed
}

func recordIngest(docID, corpus string, chunks, embeddedChunks int, embeddingFailed bool) {
	statsState.mu.Lock()
	defer statsState.mu.Unlock()

	now := time.Now().UTC()
	if statsState.ingest.ByCorpus == nil {
		statsState.ingest.ByCorpus = make(map[string]*corpusIngestStats)
	}

	statsState.ingest.DocumentsTotal++
	statsState.ingest.ChunksTotal += chunks
	statsState.ingest.EmbeddedChunks += embeddedChunks
	statsState.ingest.LastIngestAt = now

	if embeddingFailed {
		statsState.ingest.EmbeddingFailures++
	}

	corpusKey := corpus
	if corpusKey == "" {
		corpusKey = "_default"
	}
	entry := statsState.ingest.ByCorpus[corpusKey]
	if entry == nil {
		entry = &corpusIngestStats{}
		statsState.ingest.ByCorpus[corpusKey] = entry
	}
	entry.DocumentsTotal++
	entry.ChunksTotal += chunks
	entry.EmbeddedChunks += embeddedChunks
	if embeddingFailed {
		entry.EmbeddingFailures++
	}
	entry.LastIngestAt = now
	entry.LastIngestDocument = docID

	statsState.manifest.UpdatedAt = now
	_ = writeManifestToStore(statsState.manifest)
	_ = writeIngestStatsToStore(statsState.ingest)
}

func statsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, ragAgent.Stats())
}

func currentStatsSnapshot() statsSnapshot {
	statsState.mu.RLock()
	defer statsState.mu.RUnlock()

	snapshot := statsSnapshot{
		Manifest: statsState.manifest,
		Ingest:   statsState.ingest,
	}

	warnings := make([]string, 0)
	if statsState.manifest.EmbeddingModel != "" && llmConfig.EmbeddingModel != "" && statsState.manifest.EmbeddingModel != llmConfig.EmbeddingModel {
		warnings = append(warnings, fmt.Sprintf("embedding_model mismatch: indexed=%q runtime=%q", statsState.manifest.EmbeddingModel, llmConfig.EmbeddingModel))
	}
	if statsState.manifest.EmbeddingsEnabled != llmConfig.EmbeddingsEnabled {
		warnings = append(warnings, fmt.Sprintf("embeddings_enabled mismatch: indexed=%t runtime=%t", statsState.manifest.EmbeddingsEnabled, llmConfig.EmbeddingsEnabled))
	}
	if statsState.manifest.ChunkingVersion != chunkingVersion() {
		warnings = append(warnings, fmt.Sprintf("chunking_version mismatch: indexed=%q runtime=%q", statsState.manifest.ChunkingVersion, chunkingVersion()))
	}
	if statsState.manifest.LexicalEngine != "" && storeCfg.LexicalEngine != "" && statsState.manifest.LexicalEngine != storeCfg.LexicalEngine {
		warnings = append(warnings, fmt.Sprintf("lexical_engine mismatch: indexed=%q runtime=%q", statsState.manifest.LexicalEngine, storeCfg.LexicalEngine))
	}

	snapshot.Compatibility.Compatible = len(warnings) == 0
	snapshot.Compatibility.Warnings = warnings
	return snapshot
}

func readManifestFromStore() (indexManifest, error) {
	data, err := chunkStore.Get(indexManifestKey)
	if err != nil {
		if errors.Is(err, f4kvs.ErrNotFound) {
			m := defaultManifest()
			return m, writeManifestToStore(m)
		}
		return indexManifest{}, err
	}
	var m indexManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return indexManifest{}, fmt.Errorf("invalid stored manifest: %w", err)
	}
	return m, nil
}

func readIngestStatsFromStore() (ingestStats, error) {
	data, err := chunkStore.Get(ingestStatsKey)
	if err != nil {
		if errors.Is(err, f4kvs.ErrNotFound) {
			s := defaultIngestStats()
			return s, writeIngestStatsToStore(s)
		}
		return ingestStats{}, err
	}
	var s ingestStats
	if err := json.Unmarshal(data, &s); err != nil {
		return ingestStats{}, fmt.Errorf("invalid stored ingest stats: %w", err)
	}
	if s.ByCorpus == nil {
		s.ByCorpus = make(map[string]*corpusIngestStats)
	}
	return s, nil
}

func writeManifestToStore(m indexManifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return chunkStore.Put(indexManifestKey, data)
}

func writeIngestStatsToStore(s ingestStats) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return chunkStore.Put(ingestStatsKey, data)
}
