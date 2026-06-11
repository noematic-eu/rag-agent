package main

import (
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/model"
)

const (
	embedKeyPrefix = "embed:"
	embedMetaKey   = "embed:meta"
)

type EmbedRecord struct {
	ChunkID   string    `json:"chunk_id"`
	DocID     string    `json:"doc_id"`
	Corpus    string    `json:"corpus"`
	Embedding []float64 `json:"embedding"`
}

type embedMeta struct {
	Version int       `json:"version"`
	Count   int       `json:"count"`
	Dim     int       `json:"dim"`
	Model   string    `json:"model,omitempty"`
	BuiltAt time.Time `json:"built_at"`
}

func embedKey(chunkID string) string {
	return embedKeyPrefix + chunkID
}

func storeEmbedRecord(chunk model.Chunk) error {
	if chunkStore == nil || len(chunk.Embedding) == 0 {
		return nil
	}
	key := embedKey(chunk.Metadata.ChunkID)
	_, err := chunkStore.Get(key)
	if err != nil && !errors.Is(err, f4kvs.ErrNotFound) {
		return err
	}
	alreadyExists := err == nil

	rec := EmbedRecord{
		ChunkID:   chunk.Metadata.ChunkID,
		DocID:     chunk.Metadata.DocID,
		Corpus:    chunk.Metadata.Corpus,
		Embedding: chunk.Embedding,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := chunkStore.Put(key, data); err != nil {
		return err
	}
	if alreadyExists {
		return nil
	}
	return bumpEmbedMeta(len(chunk.Embedding))
}

func bumpEmbedMeta(dim int) error {
	meta, ok, err := loadEmbedMeta()
	if err != nil {
		return err
	}
	if !ok {
		meta = embedMeta{Version: 1, Dim: dim, Model: llmConfig.EmbeddingModel}
	}
	meta.Count++
	if meta.Dim == 0 {
		meta.Dim = dim
	}
	meta.BuiltAt = time.Now().UTC()
	return saveEmbedMeta(meta)
}

func deleteEmbedRecord(chunkID string) error {
	if chunkStore == nil || chunkID == "" {
		return nil
	}
	if err := chunkStore.Delete(embedKey(chunkID)); err != nil && !errors.Is(err, f4kvs.ErrNotFound) {
		return err
	}
	meta, ok, err := loadEmbedMeta()
	if err != nil || !ok {
		return err
	}
	if meta.Count > 0 {
		meta.Count--
	}
	return saveEmbedMeta(meta)
}

func loadEmbedMeta() (embedMeta, bool, error) {
	if chunkStore == nil {
		return embedMeta{}, false, nil
	}
	data, err := chunkStore.Get(embedMetaKey)
	if err != nil {
		if errors.Is(err, f4kvs.ErrNotFound) {
			return embedMeta{}, false, nil
		}
		return embedMeta{}, false, err
	}
	var meta embedMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return embedMeta{}, false, err
	}
	return meta, true, nil
}

func saveEmbedMeta(meta embedMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return chunkStore.Put(embedMetaKey, data)
}

func maybeMigrateEmbedsFromChunks() error {
	if chunkStore == nil || !llmConfig.EmbeddingsEnabled {
		return nil
	}
	if _, ok, err := loadEmbedMeta(); err != nil {
		return err
	} else if ok {
		return nil
	}

	start := time.Now()
	count := 0
	dim := 0
	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		var chunk model.Chunk
		if err := json.Unmarshal(pair.Value, &chunk); err != nil {
			continue
		}
		if len(chunk.Embedding) == 0 {
			continue
		}
		if err := storeEmbedRecord(chunk); err != nil {
			return err
		}
		count++
		if dim == 0 {
			dim = len(chunk.Embedding)
		}
	}
	if count == 0 {
		return nil
	}
	log.Printf("embed migration: %d records in %s", count, time.Since(start).Round(time.Millisecond))
	return nil
}

func scanEmbedRecords(yield func(EmbedRecord) error) error {
	if chunkStore == nil {
		return nil
	}
	pairs, err := chunkStore.ScanPrefix(embedKeyPrefix)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if pair.Key == embedMetaKey {
			continue
		}
		var rec EmbedRecord
		if err := json.Unmarshal(pair.Value, &rec); err != nil {
			continue
		}
		if len(rec.Embedding) == 0 {
			continue
		}
		if err := yield(rec); err != nil {
			return err
		}
	}
	return nil
}
