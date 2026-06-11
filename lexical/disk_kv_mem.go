package lexical

import (
	"sort"
	"sync"
)

type mapKV struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMapKV() *mapKV {
	return &mapKV{data: make(map[string][]byte)}
}

func (m *mapKV) Put(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[key] = cp
	return nil
}

func (m *mapKV) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, errMapKVNotFound
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (m *mapKV) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mapKV) ScanPrefix(prefix string) ([]KVPair, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var pairs []KVPair
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			cp := make([]byte, len(v))
			copy(cp, v)
			pairs = append(pairs, KVPair{Key: k, Value: cp})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	return pairs, nil
}

type mapKVNotFound struct{}

func (mapKVNotFound) Error() string { return "not found" }

var errMapKVNotFound = mapKVNotFound{}
