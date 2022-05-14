package chord

type KV interface {
	Put(key, value []byte) error
	Get(key []byte) (value []byte, err error)
	Delete(key []byte) error

	FindKeys(start uint64) ([][]byte, error)
}
