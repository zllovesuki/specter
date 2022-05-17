package concurrent

import (
	"sync"

	"github.com/zhangyunhao116/skipmap"
)

type KeyedRWMutex struct {
	noCopy

	mutexes *skipmap.StringMap
}

func NewKeyedRWMutex() *KeyedRWMutex {
	return &KeyedRWMutex{
		mutexes: skipmap.NewString(),
	}
}

func (m *KeyedRWMutex) obtain(key string) *sync.RWMutex {
	value, _ := m.mutexes.LoadOrStoreLazy(key, func() interface{} {
		return &sync.RWMutex{}
	})
	return value.(*sync.RWMutex)
}

func (m *KeyedRWMutex) Lock(key string) func() {
	mu := m.obtain(key)
	mu.Lock()

	return func() { mu.Unlock() }
}

func (m *KeyedRWMutex) RLock(key string) func() {
	mu := m.obtain(key)
	mu.RLock()

	return func() { mu.RUnlock() }
}
