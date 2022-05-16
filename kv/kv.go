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

func (m *MemoryMap) put(key, value []byte) {
	sKey := string(key)
	p := m.hashFn(sKey)
	if _, pOK := m.store[p]; !pOK {
		m.store[p] = make(map[string][]byte)
	}
	m.store[p][sKey] = value
}

func (m *MemoryMap) get(key []byte) []byte {
	sKey := string(key)
	p := m.hashFn(sKey)
	if s, pOK := m.store[p]; pOK {
		return s[sKey]
	}
	return nil
}

func (m *MemoryMap) delete(key []byte) {
	sKey := string(key)
	p := m.hashFn(sKey)
	if s, pOK := m.store[p]; pOK {
		delete(s, sKey)
		if len(m.store[p]) == 0 {
			delete(m.store, p)
		}
	}
}

func (m *MemoryMap) Put(key, value []byte) error {
	// TODO: concurrency
	m.mu.Lock()
	defer m.mu.Unlock()

	m.put(key, value)
	return nil
}

func (m *MemoryMap) Get(key []byte) ([]byte, error) {
	// TODO: concurrency
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.get(key), nil
}

func (m *MemoryMap) Delete(key []byte) error {
	// TODO: concurrency
	m.mu.Lock()
	defer m.mu.Unlock()

	m.delete(key)
	return nil
}

func (m *MemoryMap) LocalKeys(low, high uint64) ([][]byte, error) {
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

func (m *MemoryMap) LocalPuts(keys, values [][]byte) error {
	// TODO: concurrency
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, key := range keys {
		m.put(key, values[i])
	}
	return nil
}

func (m *MemoryMap) LocalGets(keys [][]byte) ([][]byte, error) {
	// TODO: concurrency
	m.mu.Lock()
	defer m.mu.Unlock()

	vals := make([][]byte, len(keys))
	for i, key := range keys {
		vals[i] = m.get(key)
	}
	return vals, nil
}

func (m *MemoryMap) LocalDeletes(keys [][]byte) error {
	// TODO: concurrency
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, key := range keys {
		m.delete(key)
	}
	return nil
}

func (m *MemoryMap) Fsck(low, self uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id := range m.store {
		if !chord.Between(low, id, self, true) {
			return false
		}
	}

	return true
}
