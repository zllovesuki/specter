package chord

import (
	"kon.nect.sh/specter/spec/chord"

	"go.uber.org/zap"
)

func (n *LocalNode) ownershipCheck(id uint64) error {
	if n.surrogate != nil && chord.Between(n.ID(), id, n.surrogate.GetId(), true) {
		n.Logger.Debug("KV Ownership moved", zap.Uint64("id", id), zap.Uint64("surrogate", n.surrogate.GetId()))
		return chord.ErrKVStaleOwnership
	}
	return nil
}

func errorRewrite(err error) error {
	// if the remote node happens to be leaving, the caller needs to retry
	if err == chord.ErrNodeGone {
		return chord.ErrKVStaleOwnership
	}
	return err
}

func (n *LocalNode) Put(key, value []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return errorRewrite(err)
	}
	if succ.ID() == n.ID() {
		n.Logger.Debug("KV Put", zap.String("key", string(key)), zap.Uint64("id", id))
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
		return nil, errorRewrite(err)
	}
	if succ.ID() == n.ID() {
		n.Logger.Debug("KV Get", zap.String("key", string(key)), zap.Uint64("id", id))
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
		return errorRewrite(err)
	}
	if succ.ID() == n.ID() {
		n.Logger.Debug("KV Delete", zap.String("key", string(key)), zap.Uint64("id", id))
		n.surrogateMu.RLock()
		defer n.surrogateMu.RUnlock()
		if err := n.ownershipCheck(id); err != nil {
			return err
		}
		return n.kv.Delete(key)
	}
	return succ.Delete(key)
}

func (n *LocalNode) Import(keys, values [][]byte) error {
	n.Logger.Debug("KV Import", zap.Int("num_keys", len(keys)))
	if !n.isRunning.Load() {
		return chord.ErrNodeGone
	}
	return n.kv.Import(keys, values)
}

// these operations are designed for key transfers and only used locally
func (n *LocalNode) Export(keys [][]byte) ([][]byte, error) {
	n.Logger.Debug("KV Export", zap.Int("num_keys", len(keys)))
	return n.kv.Export(keys)
}

func (n *LocalNode) RangeKeys(low, high uint64) ([][]byte, error) {
	n.Logger.Debug("KV RangeKeys", zap.Uint64("low", low), zap.Uint64("high", high))
	return n.kv.RangeKeys(low, high)
}

func (n *LocalNode) RemoveKeys(keys [][]byte) error {
	n.Logger.Debug("KV RemoveKeys", zap.Int("num_keys", len(keys)))
	return n.kv.RemoveKeys(keys)
}
