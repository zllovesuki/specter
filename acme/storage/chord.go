package storage

import (
	"context"
	"fmt"
	"io/fs"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"kon.nect.sh/specter/spec/chord"

	"github.com/caddyserver/certmagic"
	"github.com/zhangyunhao116/skipmap"
)

const (
	kvKeyPrefix = "/acme-storage/"
)

type ChordStorage struct {
	Logger *zap.Logger
	KV     chord.KV

	leaseToken     *skipmap.StringMap[*leaseHolder]
	retryInterval  time.Duration
	leaseTTL       time.Duration
	renewalInteval time.Duration
	pollInterval   time.Duration
}

type Config struct {
	RetryInterval time.Duration
	LeaseTTL      time.Duration
}

func New(logger *zap.Logger, kv chord.KV, cfg Config) (*ChordStorage, error) {
	// TODO: assert sensible interval
	return &ChordStorage{
		Logger:         logger,
		KV:             kv,
		leaseToken:     skipmap.NewString[*leaseHolder](),
		retryInterval:  cfg.RetryInterval,
		leaseTTL:       cfg.LeaseTTL,
		renewalInteval: cfg.LeaseTTL / 4,
		pollInterval:   cfg.LeaseTTL / 2,
	}, nil
}

func (c *ChordStorage) Lock(ctx context.Context, key string) error {
	// c.Logger.Debug("Lock invoked", zap.String("key", key))
	for {
		token, err := retrier(ctx, c.retryInterval, func() (uint64, error) {
			return c.KV.Acquire([]byte(keyName(key)), c.leaseTTL)
		})
		switch err {
		case chord.ErrKVLeaseConflict:
			c.Logger.Debug("Lease acquire conflict, retrying", zap.String("key", key))
			<-time.After(c.pollInterval)
			continue
		case nil:
			c.Logger.Debug("Lease acquired", zap.String("key", key), zap.Uint64("lease", token))
			h := &leaseHolder{
				token:   token,
				closeCh: make(chan struct{}),
			}
			c.leaseToken.Store(key, h)
			go c.renewLease(key, h)
			return nil
		default:
			c.Logger.Error("Error acquiring lease", zap.String("key", key), zap.Error(err))
			return err
		}
	}
}

func (c *ChordStorage) renewLease(key string, l *leaseHolder) {
	ticker := time.NewTicker(c.renewalInteval)
	defer ticker.Stop()
	for {
		select {
		case <-l.closeCh:
			return
		case <-ticker.C:
			prev := atomic.LoadUint64(&l.token)
			next, err := retrier(context.Background(), c.retryInterval, func() (uint64, error) {
				return c.KV.Renew([]byte(keyName(key)), c.leaseTTL, prev)
			})
			if err != nil {
				c.Logger.Error("failed to renew lease", zap.String("lease", key), zap.Error(err))
				return
			}
			c.Logger.Debug("Lease renewal", zap.String("key", key), zap.Uint64("newToken", next))
			atomic.StoreUint64(&l.token, next)
		}
	}
}

func (c *ChordStorage) Unlock(ctx context.Context, key string) error {
	// c.Logger.Debug("Unlock invoked", zap.String("key", key))
	lease, ok := c.leaseToken.LoadAndDelete(key)
	if !ok {
		return fmt.Errorf("not a lease holder of key %s", key)
	}
	close(lease.closeCh)
	_, err := retrier(ctx, c.retryInterval, func() (any, error) {
		return nil, c.KV.Release([]byte(keyName(key)), atomic.LoadUint64(&lease.token))
	})
	return err
}

func (c *ChordStorage) Store(ctx context.Context, key string, value []byte) error {
	// c.Logger.Debug("Store invoked", zap.String("key", key))
	_, err := retrier(ctx, c.retryInterval, func() (any, error) {
		return nil, c.KV.Put([]byte(keyName(key)), value)
	})
	return err
}

func (c *ChordStorage) Load(ctx context.Context, key string) ([]byte, error) {
	val, err := retrier(ctx, c.retryInterval, func() ([]byte, error) {
		return c.KV.Get([]byte(keyName(key)))
	})
	if err != nil {
		return nil, err
	}
	if val == nil {
		c.Logger.Debug("Load returned not found", zap.String("key", key))
		return nil, fs.ErrNotExist
	}
	c.Logger.Debug("Load returned something", zap.String("key", key), zap.Int("val_length", len(val)))
	return val, nil
}

func (c *ChordStorage) Delete(ctx context.Context, key string) error {
	// c.Logger.Debug("Delete invoked", zap.String("key", key))
	_, err := retrier(ctx, c.retryInterval, func() (any, error) {
		return nil, c.KV.Delete([]byte(keyName(key)))
	})
	return err
}

func (c *ChordStorage) Exists(ctx context.Context, key string) bool {
	// c.Logger.Debug("Exists invoked", zap.String("key", key))
	val, err := retrier(ctx, c.retryInterval, func() ([]byte, error) {
		return c.KV.Get([]byte(keyName(key)))
	})
	if err != nil {
		c.Logger.Debug("Exists error", zap.String("key", key), zap.Error(err))
		return false
	}
	c.Logger.Debug("Exists", zap.String("key", key), zap.Bool("exists", val != nil))
	return val != nil
}

func (c *ChordStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	return nil, fmt.Errorf("not yet implemented")
}

func (c *ChordStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	// c.Logger.Debug("Stat invoked", zap.String("key", key))
	info := certmagic.KeyInfo{}
	value, err := retrier(ctx, c.retryInterval, func() ([]byte, error) {
		return c.KV.Get([]byte(keyName(key)))
	})
	if err != nil {
		return info, err
	}
	if value == nil {
		return info, fs.ErrNotExist
	}
	info.IsTerminal = true
	info.Size = int64(len(value))
	info.Key = key
	return info, nil
}

type leaseHolder struct {
	closeCh chan struct{}
	token   uint64
}

var _ certmagic.Storage = (*ChordStorage)(nil)

func keyName(key string) string {
	return fmt.Sprintf("%s%s", kvKeyPrefix, key)
}

func retrier[V any](ctx context.Context, wait time.Duration, fn func() (V, error)) (V, error) {
	var zeroV V
	for {
		select {
		case <-ctx.Done():
			return zeroV, ctx.Err()
		default:
		}
		v, err := fn()
		if err != nil {
			if chord.ErrorIsRetryable(err) {
				<-time.After(wait)
				continue
			}
			return zeroV, err
		}
		return v, nil
	}
}
