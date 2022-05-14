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

func (m *MemoryMap) FindKeys(start uint64) ([][]byte, error) {
	return nil, nil
}
