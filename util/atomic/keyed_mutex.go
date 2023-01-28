package atomic

import (
	"github.com/puzpuzpuz/xsync/v2"
)

type KeyedRWMutex struct {
	noCopy
	mutexes *xsync.MapOf[string, *xsync.RBMutex]
}

func NewKeyedRWMutex() *KeyedRWMutex {
	return &KeyedRWMutex{
		mutexes: xsync.NewMapOf[*xsync.RBMutex](),
	}
}

func (m *KeyedRWMutex) obtain(key string) (mu *xsync.RBMutex) {
	mu, _ = m.mutexes.LoadOrCompute(key, func() *xsync.RBMutex {
		return xsync.NewRBMutex()
	})
	return
}

func (m *KeyedRWMutex) Lock(key string) func() {
	mu := m.obtain(key)
	mu.Lock()

	return mu.Unlock
}

func (m *KeyedRWMutex) RLock(key string) func() {
	mu := m.obtain(key)
	t := mu.RLock()

	return func() { mu.RUnlock(t) }
}
