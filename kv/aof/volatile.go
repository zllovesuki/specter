package aof

import (
	"context"
	"time"
)

func (d *DiskKV) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (token uint64, err error) {
	return d.memKv.Acquire(ctx, lease, ttl)
}

func (d *DiskKV) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	return d.memKv.Renew(ctx, lease, ttl, prevToken)
}

func (d *DiskKV) Release(ctx context.Context, lease []byte, token uint64) error {
	return d.memKv.Release(ctx, lease, token)
}
