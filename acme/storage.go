package acme

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/caddyserver/certmagic"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap"
)

type ChordStorage struct {
	Logger *zap.Logger
	KV     chord.KV

	leaseToken      *skipmap.StringMap[*leaseHolder]
	retryInterval   time.Duration
	leaseTTL        time.Duration
	renewalInterval time.Duration
	pollInterval    time.Duration
}

type StorageConfig struct {
	RetryInterval time.Duration
	LeaseTTL      time.Duration
}

func NewChordStorage(logger *zap.Logger, kv chord.KV, cfg StorageConfig) (*ChordStorage, error) {
	// TODO: assert sensible interval
	return &ChordStorage{
		Logger:          logger,
		KV:              kv,
		leaseToken:      skipmap.NewString[*leaseHolder](),
		retryInterval:   cfg.RetryInterval,
		leaseTTL:        cfg.LeaseTTL,
		renewalInterval: cfg.LeaseTTL / 4,
		pollInterval:    cfg.LeaseTTL / 2,
	}, nil
}

func (c *ChordStorage) Lock(ctx context.Context, key string) error {
	// c.Logger.Debug("Lock invoked", zap.String("key", key))
	for {
		token, err := c.KV.Acquire(ctx, []byte(kvKeyName(key)), c.leaseTTL)
		switch err {
		case chord.ErrKVLeaseConflict:
			c.Logger.Debug("Lease acquire conflict, retrying", zap.String("key", key))
			<-time.After(c.pollInterval)
			continue
		case nil:
			return c.startLeaseRenewal(key, token)
		default:
			c.Logger.Error("Error acquiring lease", zap.String("key", key), zap.Error(err))
			return err
		}
	}
}

func (c *ChordStorage) renewLease(key string, l *leaseHolder) {
	ticker := time.NewTicker(c.renewalInterval)
	defer ticker.Stop()

	defer l.Done()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			if err := c.renewLeaseOnce(context.Background(), key, l); err != nil {
				c.Logger.Error("failed to renew lease", zap.String("lease", key), zap.Error(err))
				return
			}
		}
	}
}

func (c *ChordStorage) startLeaseRenewal(key string, token uint64) error {
	leaseCtx, leaseCancel := context.WithCancel(context.Background())
	h := &leaseHolder{
		token:    token,
		ctx:      leaseCtx,
		cancelFn: leaseCancel,
	}
	h.Add(1)
	c.leaseToken.Store(key, h)
	go c.renewLease(key, h)

	c.Logger.Debug("Lease acquired", zap.String("key", key), zap.Uint64("lease", token))
	return nil
}

func (c *ChordStorage) renewLeaseOnce(ctx context.Context, key string, l *leaseHolder) error {
	prev := atomic.LoadUint64(&l.token)
	next, err := c.KV.Renew(ctx, []byte(kvKeyName(key)), c.leaseTTL, prev)
	if err != nil {
		return err
	}
	c.Logger.Debug("Lease renewal", zap.String("key", key), zap.Uint64("newToken", next))
	atomic.StoreUint64(&l.token, next)
	return nil
}

func (c *ChordStorage) RenewLockLease(ctx context.Context, key string, leaseDuration time.Duration) error {
	lease, ok := c.leaseToken.Load(key)
	if !ok {
		return fmt.Errorf("not a lease holder of key %s", key)
	}
	return c.renewLeaseOnce(ctx, key, lease)
}

func (c *ChordStorage) Unlock(ctx context.Context, key string) error {
	lease, ok := c.leaseToken.LoadAndDelete(key)
	if !ok {
		return fmt.Errorf("not a lease holder of key %s", key)
	}
	lease.cancelFn()
	lease.Wait()
	c.Logger.Debug("Lease released", zap.String("key", key))
	return c.KV.Release(ctx, []byte(kvKeyName(key)), atomic.LoadUint64(&lease.token))
}

func (c *ChordStorage) Store(ctx context.Context, key string, value []byte) error {
	// c.Logger.Debug("Store invoked", zap.String("key", key))
	return c.KV.Put(ctx, []byte(kvKeyName(key)), value)
}

func (c *ChordStorage) Load(ctx context.Context, key string) ([]byte, error) {
	val, err := c.KV.Get(ctx, []byte(kvKeyName(key)))
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
	return c.KV.Delete(ctx, []byte(kvKeyName(key)))
}

func (c *ChordStorage) Exists(ctx context.Context, key string) bool {
	// c.Logger.Debug("Exists invoked", zap.String("key", key))
	val, err := c.KV.Get(ctx, []byte(kvKeyName(key)))
	if err != nil {
		c.Logger.Debug("Exists error", zap.String("key", key), zap.Error(err))
		return false
	}
	c.Logger.Debug("Exists", zap.String("key", key), zap.Bool("exists", val != nil))
	return val != nil
}

func (c *ChordStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	oldPrefix := prefix

	prefix = kvKeyName(prefix)

	keys, err := c.KV.ListKeys(ctx, []byte(prefix))
	if err != nil {
		return nil, err
	}

	var newKey string
	found := make([]string, 0)
	if recursive {
		for _, key := range keys {
			if key.GetType() != protocol.KeyComposite_SIMPLE {
				continue
			}
			newKey = strings.TrimPrefix(string(key.GetKey()), kvKeyPrefix)
			found = append(found, newKey)
		}
	} else {
		seen := make(map[string]bool)
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		for _, key := range keys {
			if key.GetType() != protocol.KeyComposite_SIMPLE {
				continue
			}
			sub := strings.TrimPrefix(string(key.GetKey()), prefix)
			n := strings.Index(sub, "/")

			if n == -1 {
				newKey = string(key.GetKey())
			} else {
				newKey = prefix + sub[:n]
			}
			newKey = strings.TrimPrefix(newKey, kvKeyPrefix)

			if ok := seen[newKey]; ok {
				continue
			}
			seen[newKey] = true
			found = append(found, newKey)
		}
	}

	c.Logger.Debug("List", zap.String("prefix", oldPrefix), zap.Strings("keys", found), zap.Bool("recursive", recursive))

	return found, nil
}

func (c *ChordStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	// c.Logger.Debug("Stat invoked", zap.String("key", key))
	info := certmagic.KeyInfo{}
	value, err := c.KV.Get(ctx, []byte(kvKeyName(key)))
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
	sync.WaitGroup
	ctx      context.Context
	cancelFn context.CancelFunc
	token    uint64
}

var _ certmagic.Storage = (*ChordStorage)(nil)
var _ certmagic.LockLeaseRenewer = (*ChordStorage)(nil)
