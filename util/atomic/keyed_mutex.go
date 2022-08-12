package atomic

import (
	"sync"

	"github.com/zhangyunhao116/skipmap"
)

type KeyedRWMutex struct {
	noCopy
	mutexes *skipmap.StringMap[*sync.RWMutex]
}

func NewKeyedRWMutex() *KeyedRWMutex {
	return &KeyedRWMutex{
		mutexes: skipmap.NewString[*sync.RWMutex](),
	}
}

func (m *KeyedRWMutex) obtain(key string) *sync.RWMutex {
	value, _ := m.mutexes.LoadOrStoreLazy(key, func() *sync.RWMutex {
		return &sync.RWMutex{}
	})
	return value
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
