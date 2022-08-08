package memory

import "kon.nect.sh/specter/spec/chord"

var _ chord.SimpleKV = (*MemoryKV)(nil)

func (m *MemoryKV) put(key, value []byte) {
	v, _ := m.fetchVal(key)
	v.simple.Store(&value)
}

func (m *MemoryKV) get(key []byte) []byte {
	v, _ := m.fetchVal(key)
	return *v.simple.Load()
}

// only delete value from plain keyspace
func (m *MemoryKV) delete(key []byte) {
	v, _ := m.fetchVal(key)

	v.simple.Store(new([]byte))
}

func (m *MemoryKV) Put(key, value []byte) error {
	m.put(key, value)
	return nil
}

func (m *MemoryKV) Get(key []byte) ([]byte, error) {
	return m.get(key), nil
}

func (m *MemoryKV) Delete(key []byte) error {
	m.delete(key)
	return nil
}
