package chord

import (
	"kon.nect.sh/specter/spec/chord"

	"go.uber.org/zap"
)

func (n *LocalNode) Put(key, value []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
	if succ.ID() == n.ID() {
		n.Logger.Debug("KV Put", zap.String("key", string(key)), zap.Uint64("id", id), zap.Uint64("node", succ.ID()))
		return n.kv.Put(key, value)
	}
	return succ.Put(key, value)
}

func (n *LocalNode) Get(key []byte) ([]byte, error) {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return nil, err
	}
	if succ.ID() == n.ID() {
		n.Logger.Debug("KV Get", zap.String("key", string(key)), zap.Uint64("id", id), zap.Uint64("node", succ.ID()))
		return n.kv.Get(key)
	}
	return succ.Get(key)
}

func (n *LocalNode) Delete(key []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
	if succ.ID() == n.ID() {
		n.Logger.Debug("KV Delete", zap.String("key", string(key)), zap.Uint64("id", id), zap.Uint64("node", succ.ID()))
		return n.kv.Delete(key)
	}
	return succ.Delete(key)
}

func (n *LocalNode) LocalKeys(low, high uint64) ([][]byte, error) {
	n.Logger.Debug("KV LocalKeys", zap.Uint64("low", low), zap.Uint64("high", high))
	return n.kv.LocalKeys(low, high)
}

func (n *LocalNode) LocalPuts(keys, values [][]byte) error {
	n.Logger.Debug("KV LocalPuts", zap.Int("num_keys", len(keys)))
	return n.kv.LocalPuts(keys, values)
}

func (n *LocalNode) LocalGets(keys [][]byte) ([][]byte, error) {
	n.Logger.Debug("KV LocalGets", zap.Int("num_keys", len(keys)))
	return n.kv.LocalGets(keys)
}

func (n *LocalNode) LocalDeletes(keys [][]byte) error {
	n.Logger.Debug("KV LocalDeletes", zap.Int("num_keys", len(keys)))
	return n.kv.LocalDeletes(keys)
}
