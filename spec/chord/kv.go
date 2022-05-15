package chord

type KV interface {
	Put(key, value []byte) error
	Get(key []byte) (value []byte, err error)
	Delete(key []byte) error

	LocalKeys(low, high uint64) ([][]byte, error)
	LocalPuts(keys, values [][]byte) error
	LocalGets(keys [][]byte) ([][]byte, error)
	LocalDeletes(keys [][]byte) error
}
