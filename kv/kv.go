package kv

import (
	"sync/atomic"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/zhangyunhao116/skipmap"
	"github.com/zhangyunhao116/skipset"
)

type HashFn func(string) uint64

type MemoryMap struct {
	s      *skipmap.Uint64Map[*skipmap.StringMap[*kvValue]]
	hashFn HashFn
}

type kvValue struct {
	plain    atomic.Pointer[[]byte]
	children *skipset.StringSet
}

func newInnerMapFunc() *skipmap.StringMap[*kvValue] {
	return skipmap.NewString[*kvValue]()
}

func newValueFunc() *kvValue {
	v := &kvValue{
		plain:    atomic.Pointer[[]byte]{},
		children: skipset.NewString(),
	}
	v.plain.Store(new([]byte))
	return v
}

var _ chord.KV = (*MemoryMap)(nil)

func WithHashFn(fn HashFn) *MemoryMap {
	return &MemoryMap{
		s:      skipmap.NewUint64[*skipmap.StringMap[*kvValue]](),
		hashFn: fn,
	}
}

func (m *MemoryMap) fetchVal(key []byte) (*kvValue, bool) {
	sKey := string(key)
	p := m.hashFn(sKey)

	kMap, _ := m.s.LoadOrStoreLazy(p, newInnerMapFunc)
	return kMap.LoadOrStoreLazy(sKey, newValueFunc)
}

func (m *MemoryMap) put(key, value []byte) {
	v, _ := m.fetchVal(key)
	v.plain.Store(&value)
}

func (m *MemoryMap) get(key []byte) []byte {
	v, _ := m.fetchVal(key)
	return *v.plain.Load()
}

// only delete value from plain keyspace
func (m *MemoryMap) delete(key []byte) {
	v, _ := m.fetchVal(key)

	empty := *new([]byte)
	v.plain.Store(&empty)
}

// delete plain and prefix keyspaces
func (m *MemoryMap) deleteAll(key []byte) {
	sKey := string(key)
	p := m.hashFn(sKey)

	if kMap, ok := m.s.Load(p); ok {
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

func (m *MemoryMap) PrefixAppend(prefix []byte, child []byte) error {
	v, _ := m.fetchVal(prefix)

	if !v.children.Add(string(child)) {
		return chord.ErrKVPrefixConflict
	}

	return nil
}

func (m *MemoryMap) PrefixList(prefix []byte) ([][]byte, error) {
	v, _ := m.fetchVal(prefix)

	children := make([][]byte, 0)
	v.children.Range(func(value string) bool {
		children = append(children, []byte(value))
		return true
	})

	return children, nil
}

func (m *MemoryMap) PrefixRemove(prefix []byte, needle []byte) error {
	v, _ := m.fetchVal(prefix)

	v.children.Remove(string(needle))

	return nil
}

func (m *MemoryMap) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	for i, key := range keys {
		v, _ := m.fetchVal(key)
		bytes := values[i].GetPlainValue()
		v.plain.Store(&bytes)
		for _, child := range values[i].GetPrefixChildren() {
			v.children.Add(string(child))
		}
	}
	return nil
}

func (m *MemoryMap) Export(keys [][]byte) []*protocol.KVTransfer {
	vals := make([]*protocol.KVTransfer, len(keys))
	for i, key := range keys {
		children, _ := m.PrefixList(key)
		vals[i] = &protocol.KVTransfer{
			PlainValue:     m.get(key),
			PrefixChildren: children,
		}
	}
	return vals
}

func (m *MemoryMap) RangeKeys(low, high uint64) [][]byte {
	keys := make([][]byte, 0)

	m.s.Range(func(id uint64, kMap *skipmap.StringMap[*kvValue]) bool {
		if chord.Between(low, id, high, true) {
			kMap.Range(func(key string, _ *kvValue) bool {
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
		m.deleteAll(key)
	}
}

func (m *MemoryMap) Fsck(low, self uint64) bool {
	valid := true

	m.s.Range(func(id uint64, kMap *skipmap.StringMap[*kvValue]) bool {
		if !chord.Between(low, id, self, true) && kMap.Len() > 0 {
			valid = false
			return false
		}
		return true
	})
	return valid
}
