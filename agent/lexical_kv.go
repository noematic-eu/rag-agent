package main

import (
	"github.com/noematic-eu/ai-rag-agent/lexical"
)

type lexicalKVAdapter struct {
	store ChunkStore
}

func newLexicalKVAdapter(store ChunkStore) lexical.KV {
	return &lexicalKVAdapter{store: store}
}

func (a *lexicalKVAdapter) Put(key string, value []byte) error {
	return a.store.Put(key, value)
}

func (a *lexicalKVAdapter) Get(key string) ([]byte, error) {
	return a.store.Get(key)
}

func (a *lexicalKVAdapter) Delete(key string) error {
	return a.store.Delete(key)
}

func (a *lexicalKVAdapter) ScanPrefix(prefix string) ([]lexical.KVPair, error) {
	pairs, err := a.store.ScanPrefix(prefix)
	if err != nil {
		return nil, err
	}
	out := make([]lexical.KVPair, len(pairs))
	for i, pair := range pairs {
		out[i] = lexical.KVPair{Key: pair.Key, Value: pair.Value}
	}
	return out, nil
}
