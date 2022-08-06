package chord

import (
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/zap"
)

func (n *LocalNode) kvMiddleware(
	key []byte,
	value []byte,
	handler func(kv chord.KV, remote bool, id uint64, key, value []byte) ([]byte, error),
) ([]byte, error) {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	switch err {
	case nil:
	case chord.ErrNodeGone:
		// if the remote node happens to be leaving, the caller needs to retry
		return nil, chord.ErrKVStaleOwnership
	default:
		return nil, err
	}
	if succ.ID() == n.ID() {
		if !n.surrogateMu.TryRLock() {
			// this is to avoid caller timing out RPC call
			return nil, chord.ErrKVPendingTransfer
		}
		defer n.surrogateMu.RUnlock()
		if n.surrogate != nil && chord.Between(n.ID(), id, n.surrogate.GetId(), true) {
			n.Logger.Debug("KV Ownership moved", zap.Uint64("id", id), zap.Uint64("surrogate", n.surrogate.GetId()))
			return nil, chord.ErrKVStaleOwnership
		}
		return handler(n.kv, false, id, key, value)
	}
	return handler(succ, true, id, key, value)
}

func (n *LocalNode) Put(key, value []byte) error {
	_, err := n.kvMiddleware(key, value,
		func(kv chord.KV, remote bool, id uint64, key, value []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV Put", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return nil, kv.Put(key, value)
		})
	return err
}

func (n *LocalNode) Get(key []byte) ([]byte, error) {
	return n.kvMiddleware(key, nil,
		func(kv chord.KV, remote bool, id uint64, key, _ []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV Get", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return kv.Get(key)
		})
}

func (n *LocalNode) Delete(key []byte) error {
	_, err := n.kvMiddleware(key, nil,
		func(kv chord.KV, remote bool, id uint64, key, _ []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV Delete", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return nil, kv.Delete(key)
		})
	return err
}

func (n *LocalNode) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	if !n.isRunning.Load() {
		return chord.ErrNodeGone
	}
	n.Logger.Debug("KV Import", zap.Int("num_keys", len(keys)))
	return n.kv.Import(keys, values)
}

// these operations are designed for key transfers and only used locally
func (n *LocalNode) Export(keys [][]byte) []*protocol.KVTransfer {
	n.Logger.Debug("KV Export", zap.Int("num_keys", len(keys)))
	return n.kv.Export(keys)
}

func (n *LocalNode) RangeKeys(low, high uint64) [][]byte {
	n.Logger.Debug("KV RangeKeys", zap.Uint64("low", low), zap.Uint64("high", high))
	return n.kv.RangeKeys(low, high)
}

func (n *LocalNode) RemoveKeys(keys [][]byte) {
	n.Logger.Debug("KV RemoveKeys", zap.Int("num_keys", len(keys)))
	n.kv.RemoveKeys(keys)
}
