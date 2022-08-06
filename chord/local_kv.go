package chord

import (
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/zap"
)

// Workaround for https://github.com/golang/go/issues/49085#issuecomment-948108705
func kvMiddleware[V any](
	n *LocalNode,
	key []byte,
	value []byte,
	handler func(kv chord.KV, remote bool, id uint64, key, value []byte) (V, error),
) (V, error) {
	var zeroV V
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	switch err {
	case nil:
	case chord.ErrNodeGone:
		// if the remote node happens to be leaving, the caller needs to retry
		return zeroV, chord.ErrKVStaleOwnership
	default:
		return zeroV, err
	}
	if succ.ID() == n.ID() {
		if !n.surrogateMu.TryRLock() {
			// this is to avoid caller timing out RPC call
			return zeroV, chord.ErrKVPendingTransfer
		}
		defer n.surrogateMu.RUnlock()
		if n.surrogate != nil && chord.Between(n.ID(), id, n.surrogate.GetId(), true) {
			n.Logger.Debug("KV Ownership moved", zap.Uint64("id", id), zap.Uint64("surrogate", n.surrogate.GetId()))
			return zeroV, chord.ErrKVStaleOwnership
		}
		return handler(n.kv, false, id, key, value)
	}
	return handler(succ, true, id, key, value)
}

func (n *LocalNode) Put(key, value []byte) error {
	_, err := kvMiddleware(n, key, value,
		func(kv chord.KV, remote bool, id uint64, key, value []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV Put", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return nil, kv.Put(key, value)
		})
	return err
}

func (n *LocalNode) Get(key []byte) ([]byte, error) {
	return kvMiddleware(n, key, nil,
		func(kv chord.KV, remote bool, id uint64, key, _ []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV Get", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return kv.Get(key)
		})
}

func (n *LocalNode) Delete(key []byte) error {
	_, err := kvMiddleware(n, key, nil,
		func(kv chord.KV, remote bool, id uint64, key, _ []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV Delete", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return nil, kv.Delete(key)
		})
	return err
}

func (n *LocalNode) PrefixAppend(prefix []byte, child []byte) error {
	_, err := kvMiddleware(n, prefix, child,
		func(kv chord.KV, remote bool, id uint64, prefix, child []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV PrefixAppend", zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			}
			return nil, kv.PrefixAppend(prefix, child)
		})
	return err
}

func (n *LocalNode) PrefixList(prefix []byte) ([][]byte, error) {
	return kvMiddleware(n, prefix, nil,
		func(kv chord.KV, remote bool, id uint64, prefix, _ []byte) ([][]byte, error) {
			if !remote {
				n.Logger.Debug("KV PrefixList", zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			}
			return kv.PrefixList(prefix)
		})
}

func (n *LocalNode) PrefixRemove(prefix []byte, child []byte) error {
	_, err := kvMiddleware(n, prefix, child,
		func(kv chord.KV, remote bool, id uint64, prefix, child []byte) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV PrefixRemove", zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			}
			return nil, kv.PrefixRemove(prefix, child)
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
