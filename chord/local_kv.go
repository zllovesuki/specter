package chord

import (
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/zap"
)

// Workaround for https://github.com/golang/go/issues/49085#issuecomment-948108705
func kvMiddleware[V any](
	n *LocalNode,
	key []byte,
	handler func(kv chord.KV, remote bool, id uint64) (V, error),
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
		// maybe we are joining
		state := n.state.Get()
		if state != chord.Active {
			n.Logger.Debug("KV Handler node is not in active state", zap.String("key", string(key)), zap.Uint64("id", id), zap.String("state", state.String()))
			return zeroV, chord.ErrKVStaleOwnership
		}
		if !n.surrogateMu.TryRLock() {
			// this is to avoid caller timing out RPC call
			return zeroV, chord.ErrKVPendingTransfer
		}
		defer n.surrogateMu.RUnlock()
		if n.surrogate != nil && chord.Between(n.ID(), id, n.surrogate.GetId(), true) {
			n.Logger.Debug("KV Ownership moved", zap.String("key", string(key)), zap.Uint64("id", id), zap.Uint64("surrogate", n.surrogate.GetId()))
			return zeroV, chord.ErrKVStaleOwnership
		}
		return handler(n.kv, false, id)
	}
	return handler(succ, true, id)
}

func (n *LocalNode) Put(key, value []byte) error {
	_, err := kvMiddleware(n, key,
		func(kv chord.KV, remote bool, id uint64) (any, error) {
			if !remote {
				n.Logger.Debug("KV Put", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return nil, kv.Put(key, value)
		})
	return err
}

func (n *LocalNode) Get(key []byte) ([]byte, error) {
	return kvMiddleware(n, key,
		func(kv chord.KV, remote bool, id uint64) ([]byte, error) {
			if !remote {
				n.Logger.Debug("KV Get", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return kv.Get(key)
		})
}

func (n *LocalNode) Delete(key []byte) error {
	_, err := kvMiddleware(n, key,
		func(kv chord.KV, remote bool, id uint64) (any, error) {
			if !remote {
				n.Logger.Debug("KV Delete", zap.String("key", string(key)), zap.Uint64("id", id))
			}
			return nil, kv.Delete(key)
		})
	return err
}

func (n *LocalNode) PrefixAppend(prefix []byte, child []byte) error {
	_, err := kvMiddleware(n, prefix,
		func(kv chord.KV, remote bool, id uint64) (any, error) {
			if !remote {
				n.Logger.Debug("KV PrefixAppend", zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			}
			return nil, kv.PrefixAppend(prefix, child)
		})
	return err
}

func (n *LocalNode) PrefixList(prefix []byte) ([][]byte, error) {
	return kvMiddleware(n, prefix,
		func(kv chord.KV, remote bool, id uint64) ([][]byte, error) {
			if !remote {
				n.Logger.Debug("KV PrefixList", zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			}
			return kv.PrefixList(prefix)
		})
}

func (n *LocalNode) PrefixRemove(prefix []byte, child []byte) error {
	_, err := kvMiddleware(n, prefix,
		func(kv chord.KV, remote bool, id uint64) (any, error) {
			if !remote {
				n.Logger.Debug("KV PrefixRemove", zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			}
			return nil, kv.PrefixRemove(prefix, child)
		})
	return err
}

func (n *LocalNode) Acquire(lease []byte, ttl time.Duration) (uint64, error) {
	return kvMiddleware(n, lease,
		func(kv chord.KV, remote bool, id uint64) (uint64, error) {
			if !remote {
				n.Logger.Debug("KV Acquire", zap.String("lease", string(lease)), zap.Uint64("id", id))
			}
			return kv.Acquire(lease, ttl)
		})
}

func (n *LocalNode) Renew(lease []byte, ttl time.Duration, prevToken uint64) (uint64, error) {
	return kvMiddleware(n, lease,
		func(kv chord.KV, remote bool, id uint64) (uint64, error) {
			if !remote {
				n.Logger.Debug("KV Renew", zap.String("lease", string(lease)), zap.Uint64("id", id))
			}
			return kv.Renew(lease, ttl, prevToken)
		})
}

func (n *LocalNode) Release(lease []byte, token uint64) error {
	_, err := kvMiddleware(n, lease,
		func(kv chord.KV, remote bool, id uint64) (any, error) {
			if !remote {
				n.Logger.Debug("KV Release", zap.String("lease", string(lease)), zap.Uint64("id", id))
			}
			return nil, kv.Release(lease, token)
		})
	return err
}

func (n *LocalNode) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	state := n.state.Get()
	switch state {
	case chord.Inactive, chord.Leaving, chord.Left:
		return chord.ErrNodeGone
	}
	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()

	n.Logger.Debug("KV Import", zap.Int("num_keys", len(keys)))
	return n.kv.Import(keys, values)
}
