package chord

import (
	"fmt"

	"kon.nect.sh/specter/spec/chord"

	"go.uber.org/zap"
)

var (
	ErrStateOwnership = fmt.Errorf("processing node no longer has ownership over requested key")
)

func (n *LocalNode) ownershipCheck(id uint64) error {
	if n.surrogate != nil && chord.Between(n.ID(), id, n.surrogate.ID(), true) {
		fmt.Printf("(%d, %d, %d]\n", n.ID(), id, n.surrogate.ID())
		return ErrStateOwnership
	}
	return nil
}

func (n *LocalNode) Put(key, value []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
	if succ.ID() == n.ID() {
		n.Logger.Debug("KV Put", zap.String("key", string(key)), zap.Uint64("id", id), zap.Uint64("node", succ.ID()))
		n.surrogateMu.RLock()
		defer n.surrogateMu.RUnlock()
		if err := n.ownershipCheck(id); err != nil {
			return err
		}
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
		n.surrogateMu.RLock()
		defer n.surrogateMu.RUnlock()
		if err := n.ownershipCheck(id); err != nil {
			return nil, err
		}
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
		n.surrogateMu.RLock()
		defer n.surrogateMu.RUnlock()
		if err := n.ownershipCheck(id); err != nil {
			return err
		}
		return n.kv.Delete(key)
	}
	return succ.Delete(key)
}

func (n *LocalNode) LocalKeys(low, high uint64) ([][]byte, error) {
	n.Logger.Debug("KV LocalKeys", zap.Uint64("low", low), zap.Uint64("high", high), zap.Uint64("node", n.ID()))
	return n.kv.LocalKeys(low, high)
}

func (n *LocalNode) LocalPuts(keys, values [][]byte) error {
	n.Logger.Debug("KV LocalPuts", zap.Int("num_keys", len(keys)), zap.Uint64("node", n.ID()))
	return n.kv.LocalPuts(keys, values)
}

func (n *LocalNode) LocalGets(keys [][]byte) ([][]byte, error) {
	n.Logger.Debug("KV LocalGets", zap.Int("num_keys", len(keys)), zap.Uint64("node", n.ID()))
	return n.kv.LocalGets(keys)
}

func (n *LocalNode) LocalDeletes(keys [][]byte) error {
	n.Logger.Debug("KV LocalDeletes", zap.Int("num_keys", len(keys)), zap.Uint64("node", n.ID()))
	return n.kv.LocalDeletes(keys)
}
