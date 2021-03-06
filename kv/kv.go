package kv

import (
	"fmt"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

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

func (m *MemoryMap) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	for _, val := range values {
		if val.GetType() != protocol.KVValueType_SIMPLE {
			return fmt.Errorf("unsupported value type: %s", val.GetType())
		}
	}
	for i, key := range keys {
		m.put(key, values[i].GetValue())
	}
	return nil
}

func (m *MemoryMap) Export(keys [][]byte) []*protocol.KVTransfer {
	vals := make([]*protocol.KVTransfer, len(keys))
	for i, key := range keys {
		vals[i] = &protocol.KVTransfer{
			Value: m.get(key),
			Type:  protocol.KVValueType_SIMPLE,
		}
	}
	return vals
}

func (m *MemoryMap) RangeKeys(low, high uint64) [][]byte {
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

	return keys
}

func (m *MemoryMap) RemoveKeys(keys [][]byte) {
	for _, key := range keys {
		m.delete(key)
	}
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
