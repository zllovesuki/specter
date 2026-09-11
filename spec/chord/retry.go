package chord

import (
	"context"
	"expvar"
	"time"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/avast/retry-go/v5"
)

var kvRetries = expvar.NewInt("chord.kvRetries")

type retryableWrapper struct {
	VNode
	retryInterval time.Duration
	retryAttempts uint
}

// WrapRetryKV wraps a given VNode to provide automatic retry on retryable KV errors
func WrapRetryKV(vnode VNode, interval time.Duration, maxAttempts uint) VNode {
	return &retryableWrapper{
		VNode:         vnode,
		retryInterval: interval,
		retryAttempts: maxAttempts,
	}
}

func (n *retryableWrapper) retryOptions(ctx context.Context) []retry.Option {
	return []retry.Option{
		retry.Context(ctx),
		retry.Attempts(n.retryAttempts),
		retry.Delay(n.retryInterval),
		retry.OnRetry(func(n uint, err error) {
			kvRetries.Add(1)
		}),
		retry.RetryIf(ErrorIsRetryable),
		retry.LastErrorOnly(true),
	}
}

func (n *retryableWrapper) Put(ctx context.Context, key []byte, value []byte) error {
	return retry.New(n.retryOptions(ctx)...).Do(func() error {
		return n.VNode.Put(ctx, key, value)
	})
}

func (n *retryableWrapper) Get(ctx context.Context, key []byte) (value []byte, err error) {
	return retry.NewWithData[[]byte](n.retryOptions(ctx)...).Do(func() ([]byte, error) {
		return n.VNode.Get(ctx, key)
	})
}

func (n *retryableWrapper) Delete(ctx context.Context, key []byte) error {
	return retry.New(n.retryOptions(ctx)...).Do(func() error {
		return n.VNode.Delete(ctx, key)
	})
}

func (n *retryableWrapper) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	return retry.New(n.retryOptions(ctx)...).Do(func() error {
		return n.VNode.PrefixAppend(ctx, prefix, child)
	})
}

func (n *retryableWrapper) PrefixList(ctx context.Context, prefix []byte) (children [][]byte, err error) {
	return retry.NewWithData[[][]byte](n.retryOptions(ctx)...).Do(func() ([][]byte, error) {
		return n.VNode.PrefixList(ctx, prefix)
	})
}

func (n *retryableWrapper) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	return retry.NewWithData[bool](n.retryOptions(ctx)...).Do(func() (bool, error) {
		return n.VNode.PrefixContains(ctx, prefix, child)
	})
}

func (n *retryableWrapper) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	return retry.New(n.retryOptions(ctx)...).Do(func() error {
		return n.VNode.PrefixRemove(ctx, prefix, child)
	})
}

func (n *retryableWrapper) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (token uint64, err error) {
	return retry.NewWithData[uint64](n.retryOptions(ctx)...).Do(func() (uint64, error) {
		return n.VNode.Acquire(ctx, lease, ttl)
	})
}

func (n *retryableWrapper) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	return retry.NewWithData[uint64](n.retryOptions(ctx)...).Do(func() (uint64, error) {
		return n.VNode.Renew(ctx, lease, ttl, prevToken)
	})
}

func (n *retryableWrapper) Release(ctx context.Context, lease []byte, token uint64) error {
	return retry.New(n.retryOptions(ctx)...).Do(func() error {
		return n.VNode.Release(ctx, lease, token)
	})
}

func (n *retryableWrapper) ListKeys(ctx context.Context, prefix []byte) ([]*protocol.KeyComposite, error) {
	return retry.NewWithData[[]*protocol.KeyComposite](n.retryOptions(ctx)...).Do(func() ([]*protocol.KeyComposite, error) {
		return n.VNode.ListKeys(ctx, prefix)
	})
}
