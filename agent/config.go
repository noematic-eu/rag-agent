package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/lexical"
)

const (
	defaultListen  = ":8080"
	defaultDataDir = "."
)

// agentConfig holds HTTP listen address and on-disk store locations.
type agentConfig struct {
	Listen           string
	P9Addr           string
	DataDir          string
	LexicalEngine    string
	F4KVSLexicalMode string
}

func (c agentConfig) blevePath() string {
	return filepath.Join(c.DataDir, "legal.bleve")
}

func (c agentConfig) tantivyPath() string {
	return filepath.Join(c.DataDir, "legal.tantivy")
}

func (c agentConfig) chunkStorePath() string {
	return filepath.Join(c.DataDir, "legal.f4kvs")
}

func resolveF4KVSLexicalMode(flagValue string) string {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(envOr("RAG_F4KVS_LEXICAL_MODE", lexical.F4KVSLexicalModeDisk))
	}
	return lexical.ParseF4KVSLexicalMode(raw)
}

func resolveLexicalEngine(flagValue string) (string, error) {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(envOr("RAG_LEXICAL_ENGINE", "bleve"))
	}
	return lexical.ParseEngine(raw)
}

// resolveAgentConfig merges flags, environment, and defaults.
// Flag values override env when non-empty.
func resolveAgentConfig(addrFlag, p9AddrFlag, dataDirFlag, lexicalEngineFlag string) (agentConfig, error) {
	listen := strings.TrimSpace(addrFlag)
	if listen == "" {
		listen = strings.TrimSpace(envOr("RAG_LISTEN", defaultListen))
	}
	if listen == "" {
		listen = defaultListen
	}

	dataDir := strings.TrimSpace(dataDirFlag)
	if dataDir == "" {
		dataDir = strings.TrimSpace(envOr("RAG_DATA_DIR", defaultDataDir))
	}
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	dataDir = filepath.Clean(dataDir)

	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return agentConfig{}, fmt.Errorf("data-dir: %w", err)
	}
	dataDir = abs

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return agentConfig{}, fmt.Errorf("create data-dir %s: %w", dataDir, err)
	}

	p9Addr := strings.TrimSpace(p9AddrFlag)
	if p9Addr == "" {
		p9Addr = strings.TrimSpace(envOr("RAG_9P_ADDR", ""))
	}

	lexEngine, err := resolveLexicalEngine(lexicalEngineFlag)
	if err != nil {
		return agentConfig{}, err
	}

	return agentConfig{
		Listen:           listen,
		P9Addr:           p9Addr,
		DataDir:          dataDir,
		LexicalEngine:    lexEngine,
		F4KVSLexicalMode: resolveF4KVSLexicalMode(""),
	}, nil
}
