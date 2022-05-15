package concurrent

import "sync"

var (
	lockCache = sync.Pool{
		New: func() interface{} {
			return &sync.RWMutex{}
		},
	}
)

type KeyedRWMutex struct {
	noCopy

	mutexes sync.Map // Zero value is empty and ready for use
}

func (m *KeyedRWMutex) obtain(key string) *sync.RWMutex {
	n := lockCache.Get().(*sync.RWMutex)
	value, loaded := m.mutexes.LoadOrStore(key, n)
	if loaded {
		lockCache.Put(n)
	}
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
