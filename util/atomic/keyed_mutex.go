package atomic

import (
	"sync"

	"github.com/zeebo/xxh3"
)

const shardSize = 128

type KeyedRWMutex struct {
	noCopy
	mutexes [shardSize]sync.RWMutex
}

func NewKeyedRWMutex() *KeyedRWMutex {
	return &KeyedRWMutex{}
}

func (m *KeyedRWMutex) obtain(key string) *sync.RWMutex {
	i := xxh3.HashString(key) % shardSize
	return &m.mutexes[i]
}

func (m *KeyedRWMutex) Lock(key string) func() {
	mu := m.obtain(key)
	mu.Lock()

	return mu.Unlock
}

func (m *KeyedRWMutex) RLock(key string) func() {
	mu := m.obtain(key)
	mu.RLock()

	return mu.RUnlock
}
