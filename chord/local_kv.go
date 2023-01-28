package chord

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

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
	reqCtx := chord.GetRequestContext(ctx)
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

	// local KV
	sT := n.surrogateMu.RLock()
	defer n.surrogateMu.RUnlock(sT)

	pT := n.predecessorMu.RLock()
	defer n.predecessorMu.RUnlock(pT)

	var (
		surrogate = n.surrogate
		pre       = n.predecessor
		l         = n.Logger.With(
			zap.String("key", string(key)),
			zap.Uint64("id", id),
		)
	)

	if pre != nil {
		l = l.With(zap.Uint64("predecessor", pre.ID()))
	}
	if surrogate != nil {
		l = l.With(zap.Uint64("surrogate", surrogate.GetId()))
	}

	// maybe we are joining or leaving
	state := n.state.Get()
	if state != chord.Active {
		l.Debug("KV Handler node is not in active state", zap.String("state", state.String()))
		return zeroV, chord.ErrKVStaleOwnership
	}
	if surrogate != nil && chord.Between(n.ID(), id, surrogate.GetId(), true) {
		l.Debug("KV Ownership moved")
		return zeroV, chord.ErrKVStaleOwnership
	}
	if pre != nil && !chord.Between(pre.ID(), id, n.ID(), true) {
		l.Debug("Key not in range")
		return zeroV, chord.ErrKVStaleOwnership
	}

	return handler(ctx, n.kv, targetLocal, id)
}

func (n *LocalNode) Put(ctx context.Context, key, value []byte) error {
	_, err := kvMiddleware(ctx, n, key,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.Logger.Debug("KV Put", zap.String("target", target.String()), zap.String("key", string(key)), zap.Uint64("id", id))
			return nil, kv.Put(ctx, key, value)
		})
	return err
}

func (n *LocalNode) Get(ctx context.Context, key []byte) ([]byte, error) {
	return kvMiddleware(ctx, n, key,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) ([]byte, error) {
			n.Logger.Debug("KV Get", zap.String("target", target.String()), zap.String("key", string(key)), zap.Uint64("id", id))
			return kv.Get(ctx, key)
		})
}

func (n *LocalNode) Delete(ctx context.Context, key []byte) error {
	_, err := kvMiddleware(ctx, n, key,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.Logger.Debug("KV Delete", zap.String("target", target.String()), zap.String("key", string(key)), zap.Uint64("id", id))
			return nil, kv.Delete(ctx, key)
		})
	return err
}

func (n *LocalNode) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	_, err := kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.Logger.Debug("KV PrefixAppend", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return nil, kv.PrefixAppend(ctx, prefix, child)
		})
	return err
}

func (n *LocalNode) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	return kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) ([][]byte, error) {
			n.Logger.Debug("KV PrefixList", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return kv.PrefixList(ctx, prefix)
		})
}

func (n *LocalNode) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	return kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (bool, error) {
			n.Logger.Debug("KV PrefixContains", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return kv.PrefixContains(ctx, prefix, child)
		})
}

func (n *LocalNode) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	_, err := kvMiddleware(ctx, n, prefix,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.Logger.Debug("KV PrefixRemove", zap.String("target", target.String()), zap.String("prefix", string(prefix)), zap.Uint64("id", id))
			return nil, kv.PrefixRemove(ctx, prefix, child)
		})
	return err
}

func (n *LocalNode) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (uint64, error) {
	return kvMiddleware(ctx, n, lease,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (uint64, error) {
			n.Logger.Debug("KV Acquire", zap.String("target", target.String()), zap.String("lease", string(lease)), zap.Uint64("id", id))
			return kv.Acquire(ctx, lease, ttl)
		})
}

func (n *LocalNode) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (uint64, error) {
	return kvMiddleware(ctx, n, lease,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (uint64, error) {
			n.Logger.Debug("KV Renew", zap.String("target", target.String()), zap.String("lease", string(lease)), zap.Uint64("id", id))
			return kv.Renew(ctx, lease, ttl, prevToken)
		})
}

func (n *LocalNode) Release(ctx context.Context, lease []byte, token uint64) error {
	_, err := kvMiddleware(ctx, n, lease,
		func(ctx context.Context, kv chord.KV, target kvTargetType, id uint64) (any, error) {
			n.Logger.Debug("KV Release", zap.String("target", target.String()), zap.String("lease", string(lease)), zap.Uint64("id", id))
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

	n.Logger.Debug("KV Import", zap.Int("num_keys", len(keys)))
	return n.kv.Import(ctx, keys, values)
}
