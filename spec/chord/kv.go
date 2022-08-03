package chord

type KV interface {
	Put(key, value []byte) error
	Get(key []byte) (value []byte, err error)
	Delete(key []byte) error

	Import(keys, values [][]byte) error
	Export(keys [][]byte) ([][]byte, error)

	RangeKeys(low, high uint64) ([][]byte, error)
	RemoveKeys(keys [][]byte) error
}
