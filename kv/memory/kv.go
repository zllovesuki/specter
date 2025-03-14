package memory

import (
	"context"
	"strings"
	"sync/atomic"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/zhangyunhao116/skipmap"
	"github.com/zhangyunhao116/skipset"
)

type MemoryKV struct {
	s      *skipmap.Uint64Map[*skipmap.StringMap[*kvValue]]
	hashFn chord.HashFn
}

var _ chord.KVProvider = (*MemoryKV)(nil)

type kvValue struct {
	simple   atomic.Pointer[[]byte]
	lease    atomic.Uint64
	children *skipset.StringSet
}

func (v *kvValue) isDeleted() bool {
	plain := *v.simple.Load()
	children := v.children.Len()
	token := v.lease.Load()
	deleted := (plain == nil) && (children == 0) && (token == 0)
	return deleted
}

func newInnerMapFunc() *skipmap.StringMap[*kvValue] {
	return skipmap.NewString[*kvValue]()
}

func newValueFunc() *kvValue {
	v := &kvValue{
		simple:   atomic.Pointer[[]byte]{},
		children: skipset.NewString(),
		lease:    atomic.Uint64{},
	}
	var empty []byte
	v.simple.Store(&empty)
	v.lease.Store(0)
	return v
}

func WithHashFn(fn chord.HashFn) *MemoryKV {
	return &MemoryKV{
		s:      skipmap.NewUint64[*skipmap.StringMap[*kvValue]](),
		hashFn: fn,
	}
}

func (m *MemoryKV) fetchVal(key []byte) (*kvValue, bool) {
	p := m.hashFn(key)
	sKey := string(key)

	kMap, _ := m.s.LoadOrStoreLazy(p, newInnerMapFunc)
	return kMap.LoadOrStoreLazy(sKey, newValueFunc)
}

// delete plain and prefix keyspaces
func (m *MemoryKV) deleteAll(key []byte) {
	p := m.hashFn(key)
	sKey := string(key)

	if kMap, ok := m.s.Load(p); ok {
		kMap.Delete(sKey)
	}
}

func (m *MemoryKV) Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error {
	for i, key := range keys {
		v, _ := m.fetchVal(key)
		bytes := values[i].GetSimpleValue()
		v.simple.Store(&bytes)
		v.lease.Store(values[i].GetLeaseToken())
		for _, child := range values[i].GetPrefixChildren() {
			v.children.Add(string(child))
		}
	}
	return nil
}

func (m *MemoryKV) ListKeys(_ context.Context, prefix []byte) ([]*protocol.KeyComposite, error) {
	keys := make([]*protocol.KeyComposite, 0)

	m.s.Range(func(_ uint64, kMap *skipmap.StringMap[*kvValue]) bool {
		kMap.Range(func(key string, v *kvValue) bool {
			if !strings.HasPrefix(key, string(prefix)) {
				return true
			}
			if len(*v.simple.Load()) > 0 {
				keys = append(keys, &protocol.KeyComposite{
					Type: protocol.KeyComposite_SIMPLE,
					Key:  []byte(key),
				})
			}
			if v.children.Len() > 0 {
				keys = append(keys, &protocol.KeyComposite{
					Type: protocol.KeyComposite_PREFIX,
					Key:  []byte(key),
				})
			}
			if v.lease.Load() != 0 {
				keys = append(keys, &protocol.KeyComposite{
					Type: protocol.KeyComposite_LEASE,
					Key:  []byte(key),
				})
			}
			return true
		})
		return true
	})

	return keys, nil
}

func (m *MemoryKV) Export(_ context.Context, keys [][]byte) ([]*protocol.KVTransfer, error) {
	vals := make([]*protocol.KVTransfer, len(keys))
	for i, key := range keys {
		v, _ := m.fetchVal(key)
		plain := *v.simple.Load()
		children, _ := m.PrefixList(context.Background(), key)
		token := v.lease.Load()
		vals[i] = &protocol.KVTransfer{
			SimpleValue:    plain,
			PrefixChildren: children,
			LeaseToken:     token,
		}
	}
	return vals, nil
}

func (m *MemoryKV) RangeKeys(ctx context.Context, low, high uint64) ([][]byte, error) {
	keys := make([][]byte, 0)

	m.s.Range(func(id uint64, kMap *skipmap.StringMap[*kvValue]) bool {
		if chord.Between(low, id, high, true) {
			kMap.Range(func(key string, v *kvValue) bool {
				if v.isDeleted() {
					return true
				}
				keys = append(keys, []byte(key))
				return true
			})
		}
		return true
	})

	return keys, nil
}

func (m *MemoryKV) RemoveKeys(ctx context.Context, keys [][]byte) error {
	for _, key := range keys {
		m.deleteAll(key)
	}

	return nil
}
