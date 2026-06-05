package main

import (
	"encoding/json"
	"log"

	"github.com/noematic-eu/ai-rag-agent/lexical"
	"github.com/noematic-eu/ai-rag-agent/model"
)

var lexicalBackend lexical.Backend

func openLexicalBackend(cfg agentConfig) error {
	lexCfg := lexical.Config{
		DataDir: cfg.DataDir,
		Engine:  cfg.LexicalEngine,
		ScanChunks: func(yield func(model.Chunk) error) error {
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
		},
	}
	b, err := lexical.Open(lexCfg)
	if err != nil {
		return err
	}
	lexicalBackend = b
	log.Printf("lexical engine=%s", b.Engine())
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
