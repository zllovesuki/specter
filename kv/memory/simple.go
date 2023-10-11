package memory

import (
	"context"

	"go.miragespace.co/specter/spec/chord"
)

var _ chord.SimpleKV = (*MemoryKV)(nil)

var empty []byte

func (m *MemoryKV) Put(ctx context.Context, key, value []byte) error {
	v, _ := m.fetchVal(key)
	curr := v.simple.Load()
	if !v.simple.CompareAndSwap(curr, &value) {
		return chord.ErrKVSimpleConflict
	}
	return nil
}

func (m *MemoryKV) Get(ctx context.Context, key []byte) ([]byte, error) {
	v, _ := m.fetchVal(key)
	return *v.simple.Load(), nil
}

func (m *MemoryKV) Delete(ctx context.Context, key []byte) error {
	v, _ := m.fetchVal(key)
	curr := v.simple.Load()
	if !v.simple.CompareAndSwap(curr, &empty) {
		return chord.ErrKVSimpleConflict
	}
	return nil
}
