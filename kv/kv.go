package kv

import (
	"kon.nect.sh/specter/spec/chord"

	"github.com/zhangyunhao116/skipmap"
)

type HashFn func(string) uint64

type MemoryMap struct {
	s      *skipmap.Uint64Map[*skipmap.StringMap[[]byte]]
	hashFn HashFn
}

func newValFunc() *skipmap.StringMap[[]byte] {
	return skipmap.NewString[[]byte]()
}

var _ chord.KV = (*MemoryMap)(nil)

func WithChordHash() *MemoryMap {
	return &MemoryMap{
		s:      skipmap.NewUint64[*skipmap.StringMap[[]byte]](),
		hashFn: chord.HashString,
	}
}

func WithHashFn(fn HashFn) *MemoryMap {
	return &MemoryMap{
		s:      skipmap.NewUint64[*skipmap.StringMap[[]byte]](),
		hashFn: fn,
	}
}

func (m *MemoryMap) put(key, value []byte) {
	sKey := string(key)
	p := m.hashFn(sKey)

	kMap, _ := m.s.LoadOrStoreLazy(p, newValFunc)
	kMap.Store(sKey, value)
}

func (m *MemoryMap) get(key []byte) []byte {
	sKey := string(key)
	p := m.hashFn(sKey)
	kMap, _ := m.s.LoadOrStoreLazy(p, newValFunc)
	if v, ok := kMap.Load(sKey); ok {
		return v
	}
	return nil
}

func (m *MemoryMap) delete(key []byte) {
	sKey := string(key)
	p := m.hashFn(sKey)

	if kMap, ok := m.s.Load(p); ok {
		// not safe to delete the entire submap because atomic
		kMap.Delete(sKey)
	}
}

func (m *MemoryMap) Put(key, value []byte) error {
	m.put(key, value)
	return nil
}

func (m *MemoryMap) Get(key []byte) ([]byte, error) {
	return m.get(key), nil
}

func (m *MemoryMap) Delete(key []byte) error {
	m.delete(key)
	return nil
}

func (m *MemoryMap) LocalKeys(low, high uint64) ([][]byte, error) {
	keys := make([][]byte, 0)

	m.s.Range(func(key uint64, value *skipmap.StringMap[[]byte]) bool {
		if chord.Between(low, key, high, true) {
			value.Range(func(key string, value []byte) bool {
				keys = append(keys, []byte(key))
				return true
			})
		}
		return true
	})

	return keys, nil
}

func (m *MemoryMap) DirectPuts(keys, values [][]byte) error {
	for i, key := range keys {
		m.put(key, values[i])
	}
	return nil
}

func (m *MemoryMap) LocalGets(keys [][]byte) ([][]byte, error) {
	vals := make([][]byte, len(keys))
	for i, key := range keys {
		vals[i] = m.get(key)
	}
	return vals, nil
}

func (m *MemoryMap) LocalDeletes(keys [][]byte) error {
	for _, key := range keys {
		m.delete(key)
	}
	return nil
}

func (m *MemoryMap) Fsck(low, self uint64) bool {
	valid := true

	m.s.Range(func(key uint64, value *skipmap.StringMap[[]byte]) bool {
		if !chord.Between(low, key, self, true) && value.Len() > 0 {
			valid = false
			return false
		}
		return true
	})
	return valid
}
