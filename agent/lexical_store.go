package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/noematic-eu/ai-rag-agent/lexical"
	"github.com/noematic-eu/ai-rag-agent/model"
)

var (
	lexicalBackend   lexical.Backend
	lexicalRebuildMu sync.Mutex
)

// FinalizeResult reports the outcome of POST /finalize.
type FinalizeResult struct {
	Status        string  `json:"status"`
	Mode          string  `json:"mode,omitempty"` // incremental (default) or full
	ChunksIndexed int     `json:"chunks_indexed,omitempty"`
	ChunksSkipped int     `json:"chunks_skipped,omitempty"`
	ChunksTotal   int     `json:"chunks_total,omitempty"`
	DurationSec   float64 `json:"duration_s,omitempty"`
}

func openLexicalBackend(cfg agentConfig) error {
	lexCfg := lexical.Config{
		DataDir:          cfg.DataDir,
		Engine:           cfg.LexicalEngine,
		F4KVSLexicalMode: cfg.F4KVSLexicalMode,
		ScanChunks:       scanChunksFromStore,
	}
	if cfg.LexicalEngine == lexical.EngineF4KVS && cfg.F4KVSLexicalMode == lexical.F4KVSLexicalModeDisk {
		if chunkStore == nil {
			return fmt.Errorf("f4kvs disk lexical requires chunk store")
		}
		lexCfg.KV = newLexicalKVAdapter(chunkStore)
	}
	b, err := lexical.Open(lexCfg)
	if err != nil {
		return err
	}
	lexicalBackend = b
	log.Printf("lexical engine=%s mode=%s", b.Engine(), cfg.F4KVSLexicalMode)
	return nil
}

func closeLexicalBackend() error {
	if lexicalBackend == nil {
		return nil
	}
	err := lexicalBackend.Close()
	lexicalBackend = nil
	return err
}

func scanChunksFromStore(yield func(model.Chunk) error) error {
	if chunkStore == nil {
		return nil
	}
	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		var chunk model.Chunk
		if err := json.Unmarshal(pair.Value, &chunk); err != nil {
			continue
		}
		if err := yield(chunk); err != nil {
			return err
		}
	}
	return nil
}

func countIndexableChunksInStore() (int, error) {
	n := 0
	err := scanChunksFromStore(func(chunk model.Chunk) error {
		if chunk.Metadata.ChunkID != "" && chunk.Text != "" {
			n++
		}
		return nil
	})
	return n, err
}

func rebuildLexicalFromChunkStore(onProgress lexical.RebuildProgressFunc, fullRebuild bool) (FinalizeResult, error) {
	if lexicalBackend == nil || chunkStore == nil {
		return FinalizeResult{}, fmt.Errorf("stores not open")
	}
	if lexicalBackend.Engine() != lexical.EngineF4KVS {
		return FinalizeResult{Status: "finalized"}, nil
	}

	lexicalRebuildMu.Lock()
	defer lexicalRebuildMu.Unlock()

	mode := "incremental"
	if fullRebuild {
		mode = "full"
	}
	log.Printf("rebuilding f4kvs lexical index from chunk store (mode=%s)", mode)
	chunksTotal, err := countIndexableChunksInStore()
	if err != nil {
		return FinalizeResult{}, err
	}
	beginLexicalRebuild(chunksTotal)
	progress := onProgress
	if progress == nil {
		progress = lexicalRebuildProgressCallback()
	} else {
		wrapped := progress
		progress = func(indexed, total int) {
			updateLexicalRebuild(indexed, total)
			wrapped(indexed, total)
		}
	}

	if lexical.F4KVSUsesDiskMode(lexicalBackend) {
		var stats lexical.RebuildStats
		if fullRebuild {
			stats, err = lexical.RebuildF4KVSDisk(lexicalBackend, scanChunksFromStore, chunksTotal, progress)
		} else {
			stats, err = lexical.RebuildF4KVSDiskIncremental(lexicalBackend, scanChunksFromStore, chunksTotal, progress)
		}
		if err != nil {
			clearLexicalRebuildOnError()
			return FinalizeResult{}, err
		}
		finishLexicalRebuild(stats.ChunksIndexed+stats.ChunksSkipped, stats.ChunksTotal, stats.Duration)
		return FinalizeResult{
			Status:        "finalized",
			Mode:          stats.Mode,
			ChunksIndexed: stats.ChunksIndexed,
			ChunksSkipped: stats.ChunksSkipped,
			ChunksTotal:   stats.ChunksTotal,
			DurationSec:   stats.Duration.Seconds(),
		}, nil
	}
	if err := lexical.RebuildF4KVSRam(lexicalBackend, scanChunksFromStore); err != nil {
		clearLexicalRebuildOnError()
		return FinalizeResult{}, err
	}
	finishLexicalRebuild(chunksTotal, chunksTotal, 0)
	return FinalizeResult{Status: "finalized", ChunksIndexed: chunksTotal, ChunksTotal: chunksTotal}, nil
}

func maybeRebuildLexicalIfStale() {
	if lexicalBackend == nil || storeCfg.LexicalEngine != lexical.EngineF4KVS {
		return
	}
	statsState.mu.RLock()
	expected := statsState.ingest.ChunksTotal
	statsState.mu.RUnlock()
	if expected < 100 {
		return
	}
	indexed := lexical.F4KVSIndexedChunkCount(lexicalBackend)
	if indexed >= expected/2 {
		return
	}
	if _, err := rebuildLexicalFromChunkStore(nil, false); err != nil {
		log.Printf("lexical rebuild failed: %v", err)
	}
}

func f4kvsLexicalUsesRAM() bool {
	return storeCfg.LexicalEngine == lexical.EngineF4KVS &&
		storeCfg.F4KVSLexicalMode == lexical.F4KVSLexicalModeRAM
}
