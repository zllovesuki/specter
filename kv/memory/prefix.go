package memory

import (
	"context"

	"go.miragespace.co/specter/spec/chord"
)

var _ chord.PrefixKV = (*MemoryKV)(nil)

func (m *MemoryKV) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	v, _ := m.fetchVal(prefix)

	if !v.children.Add(string(child)) {
		return chord.ErrKVPrefixConflict
	}

	return nil
}

func (m *MemoryKV) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	children := make([][]byte, 0)
	v, ok := m.lookupVal(prefix)
	if !ok {
		return children, nil
	}
	v.children.Range(func(value string) bool {
		children = append(children, []byte(value))
		return true
	})

	return children, nil
}

func (m *MemoryKV) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	v, ok := m.lookupVal(prefix)
	if !ok {
		return false, nil
	}

	return v.children.Contains(string(child)), nil
}

func (m *MemoryKV) PrefixRemove(ctx context.Context, prefix []byte, needle []byte) error {
	v, _ := m.fetchVal(prefix)

	v.children.Remove(string(needle))

	return nil
}
