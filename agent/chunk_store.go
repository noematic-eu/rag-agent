package main

import (
	"encoding/json"
	"log"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/model"
)

// ChunkKVPair is a key/value entry returned by chunk store prefix scans.
type ChunkKVPair struct {
	Key   string
	Value []byte
}

// ChunkStore persists chunk payloads on disk.
type ChunkStore interface {
	Put(key string, value []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
	ScanPrefix(prefix string) ([]ChunkKVPair, error)
	Compact() error
	Close() error
}

type f4kvsChunkStore struct {
	store *f4kvs.Store
}

func (s *f4kvsChunkStore) Put(key string, value []byte) error {
	return s.store.Put(key, value)
}

func (s *f4kvsChunkStore) Get(key string) ([]byte, error) {
	return s.store.Get(key)
}

func (s *f4kvsChunkStore) Delete(key string) error {
	return s.store.Delete(key)
}

func (s *f4kvsChunkStore) ScanPrefix(prefix string) ([]ChunkKVPair, error) {
	pairs, err := s.store.ScanPrefix(prefix)
	if err != nil {
		return nil, err
	}
	out := make([]ChunkKVPair, len(pairs))
	for i, pair := range pairs {
		out[i] = ChunkKVPair{Key: pair.Key, Value: pair.Value}
	}
	return out, nil
}

func (s *f4kvsChunkStore) Compact() error {
	return s.store.Compact()
}

func (s *f4kvsChunkStore) Close() error {
	return s.store.Close()
}

var chunkStore ChunkStore

// storeDocumentMetadata stores legacy document metadata in the chunk store.
func storeDocumentMetadata(doc model.LegalDocument) {
	data, err := json.Marshal(doc)
	if err != nil {
		log.Printf("Erreur lors du marshalling du document %s : %v", doc.ID, err)
		return
	}

	if err := chunkStore.Put("doc:"+doc.ID, data); err != nil {
		log.Printf("Erreur lors du stockage des métadonnées pour %s : %v", doc.ID, err)
	}
}
