package kv

import (
	"sync"

	"specter/spec/chord"
)

type HashFn func(string) uint64

type MemoryMap struct {
	mu sync.RWMutex

	store  map[uint64]map[string][]byte
	hashFn HashFn
}

var _ chord.KV = &MemoryMap{}

func WithChordHash() *MemoryMap {
	return &MemoryMap{
		store:  make(map[uint64]map[string][]byte),
		hashFn: chord.HashString,
	}
}

func WithHashFn(fn HashFn) *MemoryMap {
	return &MemoryMap{
		store:  make(map[uint64]map[string][]byte),
		hashFn: fn,
	}
}

func (m *MemoryMap) Put(key, value []byte) error {
	// TODO: concurrency
	m.mu.Lock()
	defer m.mu.Unlock()

	sKey := string(key)
	p := m.hashFn(sKey)
	if _, pOK := m.store[p]; !pOK {
		m.store[p] = make(map[string][]byte)
	}
	m.store[p][sKey] = value
	return nil
}

func (m *MemoryMap) Get(key []byte) ([]byte, error) {
	// TODO: concurrency
	m.mu.RLock()
	defer m.mu.RUnlock()

	sKey := string(key)
	p := m.hashFn(sKey)
	if s, pOK := m.store[p]; pOK {
		return s[sKey], nil
	}
	return nil, nil
}

func (m *MemoryMap) Delete(key []byte) error {
	// TODO: concurrency
	m.mu.Lock()
	defer m.mu.Unlock()

	sKey := string(key)
	p := m.hashFn(sKey)
	if s, pOK := m.store[p]; pOK {
		delete(s, sKey)
	}
	return nil
}

func (m *MemoryMap) FindKeys(low, high uint64) ([][]byte, error) {
	// TODO: concurrency
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([][]byte, 0)
	for id := range m.store {
		if chord.Between(low, id, high, true) {
			for k := range m.store[id] {
				keys = append(keys, []byte(k))
			}
		}
	}

	return keys, nil
}
