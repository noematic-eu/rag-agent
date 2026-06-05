package main

import (
	"fmt"
	"log"
	"os"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/lexical"
)

var storeCfg agentConfig

func openChunkStore() error {
	store, err := f4kvs.Open(storeCfg.chunkStorePath())
	if err != nil {
		return fmt.Errorf("f4kvs init: %w", err)
	}
	chunkStore = &f4kvsChunkStore{store: store}
	return nil
}

func openStores(cfg agentConfig) error {
	storeCfg = cfg
	if err := openChunkStore(); err != nil {
		return err
	}
	if err := openLexicalBackend(cfg); err != nil {
		_ = chunkStore.Close()
		chunkStore = nil
		return err
	}
	if globalIDF == nil {
		globalIDF = make(map[string]float64)
	}
	if err := loadStatsFromStore(); err != nil {
		return fmt.Errorf("stats init: %w", err)
	}
	return nil
}

func resetStores() error {
	if err := closeStores(); err != nil {
		log.Printf("warning while closing stores before reset: %v", err)
	}

	for _, dir := range []string{
		storeCfg.blevePath(),
		storeCfg.tantivyPath(),
		storeCfg.chunkStorePath(),
	} {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
	}

	documentTFIDFs = make([]DocumentTFIDF, 0)
	globalIDF = make(map[string]float64)
	totalDocs = 0
	resetStatsState()

	return openStores(storeCfg)
}

func maybeCompactChunkStore() error {
	repair := os.Getenv("RAG_F4KVS_COMPACT") == "1" || os.Getenv("RAG_BADGER_REPAIR") == "1"
	if !repair || chunkStore == nil {
		return nil
	}
	log.Printf("Running f4kvs compaction (RAG_F4KVS_COMPACT=1)")
	return chunkStore.Compact()
}

func closeStores() error {
	var err error
	if lexicalBackend != nil {
		lexical.ClearF4KVSRamIndex(lexicalBackend)
	}
	if e := closeLexicalBackend(); e != nil {
		err = e
	}
	if chunkStore != nil {
		if e := chunkStore.Close(); e != nil {
			if err == nil {
				err = fmt.Errorf("f4kvs: %w", e)
			}
		}
		chunkStore = nil
	}
	return err
}
