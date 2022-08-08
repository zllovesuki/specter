package memory

import "kon.nect.sh/specter/spec/chord"

var _ chord.PrefixKV = (*MemoryKV)(nil)

func (m *MemoryKV) PrefixAppend(prefix []byte, child []byte) error {
	v, _ := m.fetchVal(prefix)

	if !v.children.Add(string(child)) {
		return chord.ErrKVPrefixConflict
	}

	return nil
}

func (m *MemoryKV) PrefixList(prefix []byte) ([][]byte, error) {
	v, _ := m.fetchVal(prefix)

	children := make([][]byte, 0)
	v.children.Range(func(value string) bool {
		children = append(children, []byte(value))
		return true
	})

	return children, nil
}

func (m *MemoryKV) PrefixRemove(prefix []byte, needle []byte) error {
	v, _ := m.fetchVal(prefix)

	v.children.Remove(string(needle))

	return nil
}
