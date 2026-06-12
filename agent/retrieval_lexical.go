package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/lexical"
)

const (
	lexicalSourceIndex   = "index"
	lexicalSourceScan    = "scan"
	defaultScanMaxChunks = 5000
)

type lexicalRetrievalConfig struct {
	mode          string   // auto, or explicit chain joined
	chain         []string // index, scan
	scanMaxChunks int
}

func parseLexicalRetrievalConfig(c *gin.Context) lexicalRetrievalConfig {
	raw := strings.TrimSpace(c.Query("retrieval_lex"))
	if raw == "" {
		raw = "auto"
	}
	cfg := lexicalRetrievalConfig{mode: strings.ToLower(raw)}
	cfg.chain = resolveLexicalChain(cfg.mode)
	cfg.scanMaxChunks = intQueryParam(c, "scan_max_chunks", 0)
	if cfg.scanMaxChunks <= 0 {
		if v := strings.TrimSpace(os.Getenv("RAG_SCAN_MAX_CHUNKS")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.scanMaxChunks = n
			}
		}
	}
	return cfg
}

func parseLexicalRetrievalFromString(raw string, scanMaxChunks int) lexicalRetrievalConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "auto"
	}
	cfg := lexicalRetrievalConfig{mode: strings.ToLower(raw), scanMaxChunks: scanMaxChunks}
	cfg.chain = resolveLexicalChain(cfg.mode)
	return cfg
}

func resolveLexicalChain(mode string) []string {
	switch mode {
	case "", "auto":
		return []string{lexicalSourceIndex, lexicalSourceScan}
	case lexicalSourceIndex:
		return []string{lexicalSourceIndex}
	case lexicalSourceScan:
		return []string{lexicalSourceScan}
	default:
		parts := strings.Split(mode, ",")
		var chain []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			switch p {
			case lexicalSourceIndex, lexicalSourceScan:
				chain = append(chain, p)
			}
		}
		if len(chain) == 0 {
			return []string{lexicalSourceIndex, lexicalSourceScan}
		}
		return chain
	}
}

func effectiveScanMaxChunks(corpus string, configured int) int {
	if configured > 0 {
		return configured
	}
	if corpus != "" {
		return 0
	}
	return defaultScanMaxChunks
}

func lexicalIndexRebuilding() bool {
	if lexicalBackend == nil {
		return false
	}
	return lexical.F4KVSIsRebuilding(lexicalBackend)
}

func retrieveLexicalHits(text, corpus string, k int, cfg lexicalRetrievalConfig) ([]lexical.Hit, string, error) {
	if k <= 0 {
		return nil, "", nil
	}
	if len(cfg.chain) == 0 {
		cfg = parseLexicalRetrievalFromString("auto", cfg.scanMaxChunks)
	}
	if cfg.mode == "auto" && lexicalIndexRebuilding() {
		hits, err := retrieveLexicalScan(text, corpus, k, cfg)
		if err != nil {
			return nil, "", err
		}
		if len(hits) > 0 {
			return hits, lexicalSourceScan, nil
		}
	}

	for i, step := range cfg.chain {
		var hits []lexical.Hit
		var err error
		switch step {
		case lexicalSourceIndex:
			hits, err = retrieveLexicalIndex(text, corpus, k)
		case lexicalSourceScan:
			hits, err = retrieveLexicalScan(text, corpus, k, cfg)
		default:
			continue
		}
		if err != nil {
			return nil, "", err
		}
		if len(hits) > 0 {
			return hits, step, nil
		}
		if cfg.mode == "auto" && step == lexicalSourceIndex && i+1 < len(cfg.chain) {
			continue
		}
	}
	return nil, "", nil
}

func retrieveLexicalIndex(text, corpus string, k int) ([]lexical.Hit, error) {
	if lexicalBackend == nil {
		return nil, nil
	}
	return lexicalBackend.Search(text, corpus, k)
}

func retrieveLexicalScan(text, corpus string, k int, cfg lexicalRetrievalConfig) ([]lexical.Hit, error) {
	if chunkStore == nil {
		return nil, nil
	}
	maxChunks := effectiveScanMaxChunks(corpus, cfg.scanMaxChunks)
	if corpus == "" && maxChunks > 0 {
		log.Printf("retrieval: scan BM25 capped at %d chunks (no corpus filter); set corpus= or scan_max_chunks=0 to scan all", maxChunks)
	}
	return lexical.SearchBM25Scan(scanChunksFromStore, text, corpus, k, maxChunks)
}

func applyLexicalSourceMeta(meta map[string]string, source string) {
	if meta == nil || source == "" {
		return
	}
	meta["retrieval_lex_source"] = source
}
