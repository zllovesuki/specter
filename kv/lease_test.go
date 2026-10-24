package kv

import (
	"math"
	"testing"
	"testing/synctest"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
)

func TestLeaseRenewalAcrossProviders(t *testing.T) {
	// AOF delegates leases to memory; its pooled WAL channels must stay outside synctest.
	for _, provider := range []string{"memory", "sqlite"} {
		t.Run(provider, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store, _ := transferProvider(t, provider, t.TempDir())
				ctx := t.Context()
				key := []byte("lease")
				initial, err := store.Acquire(ctx, key, 2*time.Second)
				require.NoError(t, err)

				// A second renewal must not restore a token used before the first.
				current := initial
				for range 2 {
					next, err := store.Renew(ctx, key, 2*time.Second, current)
					require.NoError(t, err)
					require.Greater(t, next, current)
					current = next
				}
				next, err := store.Renew(ctx, key, time.Second, current)
				require.NoError(t, err)
				require.Greater(t, next, current, "a shorter TTL must not shorten the lease")
				current = next

				_, err = store.Renew(ctx, key, time.Second, initial)
				require.ErrorIs(t, err, chord.ErrKVLeaseExpired)
				require.ErrorIs(t, store.Release(ctx, key, initial), chord.ErrKVLeaseExpired)

				// An unrepresentable expiry must leave the current lease intact.
				_, err = store.Renew(ctx, key, time.Duration(math.MaxInt64), current)
				require.ErrorIs(t, err, chord.ErrKVLeaseInvalidTTL)
				require.NoError(t, store.Release(ctx, key, current))
				_, err = store.Acquire(ctx, key, time.Duration(math.MaxInt64))
				require.ErrorIs(t, err, chord.ErrKVLeaseInvalidTTL)
				current, err = store.Acquire(ctx, key, time.Second)
				require.NoError(t, err)
				require.NoError(t, store.Release(ctx, key, current))

				// Expired leases and tokens with no representable successor cannot renew.
				for _, token := range []uint64{uint64(time.Now().UnixNano()), math.MaxInt64} {
					require.NoError(t, store.Import(ctx, [][]byte{key}, []*protocol.KVTransfer{{LeaseToken: token}}))
					_, err = store.Renew(ctx, key, time.Second, token)
					require.ErrorIs(t, err, chord.ErrKVLeaseExpired)
					require.NoError(t, store.Release(ctx, key, token))
				}
			})
		})
	}
}
