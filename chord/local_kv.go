package chord

import (
	"context"
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"go.uber.org/zap"
)

// Workaround for https://github.com/golang/go/issues/49085#issuecomment-948108705
func kvMiddleware[V any](
	ctx context.Context,
	n *LocalNode,
	key []byte,
	handler func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (V, error),
) (V, error) {
	var zeroV V
	id := chord.Hash(key)
	reqCtx := rpc.GetContext(ctx)
	// if it is a replication request, bypass ownership checks
	if reqCtx.GetRequestTarget() == protocol.Context_KV_REPLICATION {
		return handler(ctx, n.kv, targetReplication, id)
	}
	// otherwise, continue with the usual lookup + forward/handle
	succ, err := n.FindSuccessor(id)
	switch err {
	case nil:
	case chord.ErrNodeGone:
		// if the remote node happens to be leaving, the caller needs to retry
		return zeroV, chord.ErrKVStaleOwnership
	default:
		return zeroV, err
	}

	if succ.ID() != n.ID() {
		// remote KV
		return handler(ctx, succ, targetRemote, id)
	}

	l := n.logger.With(
		zap.String("key", string(key)),
		zap.Uint64("id", id),
	)

	// local KV
	n.surrogateMu.RLock()
	defer n.surrogateMu.RUnlock()

	// maybe we are joining or leaving
	state := n.state.Get()
	if state != chord.Active {
		l.Debug("KV Handler node is not in active state", zap.String("state", state.String()))
		n.kvStaleCount.Inc()
		return zeroV, chord.ErrKVStaleOwnership
	}

	if n.surrogate != nil {
		l = l.With(zap.Object("surrogate", n.surrogate))
	}

	n.predecessorMu.RLock()
	defer n.predecessorMu.RUnlock()

	if n.predecessor != nil {
		l = l.With(zap.Object("predecessor", n.predecessor.Identity()))
	}

	if n.surrogate != nil && chord.Between(n.ID(), id, n.surrogate.GetId(), true) {
		l.Debug("KV Ownership moved")
		n.kvStaleCount.Inc()
		return zeroV, chord.ErrKVStaleOwnership
	}

	if n.predecessor != nil && !chord.Between(n.predecessor.ID(), id, n.ID(), true) {
		l.Debug("Key not in range")
		n.kvStaleCount.Inc()
		return zeroV, chord.ErrKVStaleOwnership
	}

	return handler(ctx, n.kv, targetLocal, id)
}

func (n *LocalNode) Put(ctx context.Context, key, value []byte) error {
	_, err := kvMiddleware(ctx, n, key,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.logger.Debug("KV Put", zap.String("target", target.String()), zap.String("key", string(key)), zap.Uint64("id", id))
			return nil, kv.Put(ctx, key, value)
		})
	return err
}

func (n *LocalNode) Get(ctx context.Context, key []byte) ([]byte, error) {
	return kvMiddleware(ctx, n, key,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) ([]byte, error) {
			n.logger.Debug("KV Get", zap.String("target", target.String()), zap.String("key", string(key)), zap.Uint64("id", id))
			return kv.Get(ctx, key)
		})
}

func (n *LocalNode) Delete(ctx context.Context, key []byte) error {
	_, err := kvMiddleware(ctx, n, key,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.logger.Debug("KV Delete", zap.String("target", target.String()), zap.String("key", string(key)), zap.Uint64("id", id))
			return nil, kv.Delete(ctx, key)
		})
	return err
}

func (n *LocalNode) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	_, err := kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.logger.Debug("KV PrefixAppend", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return nil, kv.PrefixAppend(ctx, prefix, child)
		})
	return err
}

func (n *LocalNode) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	return kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) ([][]byte, error) {
			n.logger.Debug("KV PrefixList", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return kv.PrefixList(ctx, prefix)
		})
}

func (n *LocalNode) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	return kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (bool, error) {
			n.logger.Debug("KV PrefixContains", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return kv.PrefixContains(ctx, prefix, child)
		})
}

func (n *LocalNode) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	_, err := kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.logger.Debug("KV PrefixRemove", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return nil, kv.PrefixRemove(ctx, prefix, child)
		})
	return err
}

func (n *LocalNode) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (uint64, error) {
	return kvMiddleware(ctx, n, lease,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (uint64, error) {
			n.logger.Debug("KV Acquire", zap.String("target", target.String()), zap.String("lease", string(lease)), zap.Uint64("id", id))
			return kv.Acquire(ctx, lease, ttl)
		})
}

func (n *LocalNode) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (uint64, error) {
	return kvMiddleware(ctx, n, lease,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (uint64, error) {
			n.logger.Debug("KV Renew", zap.String("target", target.String()), zap.String("lease", string(lease)), zap.Uint64("id", id))
			return kv.Renew(ctx, lease, ttl, prevToken)
		})
}

func (n *LocalNode) Release(ctx context.Context, lease []byte, token uint64) error {
	_, err := kvMiddleware(ctx, n, lease,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.logger.Debug("KV Release", zap.String("target", target.String()), zap.String("lease", string(lease)), zap.Uint64("id", id))
			return nil, kv.Release(ctx, lease, token)
		})
	return err
}

func (n *LocalNode) Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error {
	state := n.state.Get()
	switch state {
	case chord.Inactive, chord.Leaving, chord.Left:
		return chord.ErrNodeGone
	}
	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()

	n.logger.Debug("KV Import", zap.Int("num_keys", len(keys)))
	return n.kv.Import(ctx, keys, values)
}

func (n *LocalNode) ListKeys(ctx context.Context, prefix []byte) ([]*protocol.KeyComposite, error) {
	reqCtx := rpc.GetContext(ctx)
	if reqCtx.GetRequestTarget() == protocol.Context_KV_DIRECT_TARGET {
		n.logger.Debug("KV Listkeys", zap.Stringer("target", targetLocal), zap.String("prefix", string(prefix)))
		return func() ([]*protocol.KeyComposite, error) {
			n.surrogateMu.RLock()
			defer n.surrogateMu.RUnlock()

			state := n.state.Get()
			if state != chord.Active {
				n.kvStaleCount.Inc()
				return nil, chord.ErrKVStaleOwnership
			}

			return n.kv.ListKeys(ctx, prefix)
		}()
	}
	n.logger.Debug("KV Listkeys", zap.Stringer("target", targetRemote), zap.String("prefix", string(prefix)))

	var (
		keys              = make([]*protocol.KeyComposite, 0)
		nodes             = make([]chord.VNode, 0)
		seen              = make(map[uint64]bool)
		next  chord.VNode = n
		err   error
	)

	// we need to walk the entire ring
	for {
		next, err = n.FindSuccessor(chord.ModuloSum(next.ID(), 1))
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, chord.ErrNodeNoSuccessor
		}
		if next.ID() == n.ID() {
			nodes = append(nodes, n)
			break
		}
		if seen[next.ID()] {
			return nil, fmt.Errorf("ring is unstable")
		}
		nodes = append(nodes, next)
		seen[next.ID()] = true
	}

	listCtx := rpc.WithContext(ctx, &protocol.Context{
		RequestTarget: protocol.Context_KV_DIRECT_TARGET,
	})
	for _, node := range nodes {
		k, err := node.ListKeys(listCtx, prefix)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k...)
	}

	return keys, nil
}
