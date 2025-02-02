package sqlite3

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquireMutualExclusion(t *testing.T) {
	as := require.New(t)

	kv := testGetKV(t)

	key := make([]byte, 8)
	rand.Read(key)

	token, err := kv.Acquire(context.Background(), key, time.Second)
	as.NoError(err)

	_, err = kv.Acquire(context.Background(), key, time.Second)
	as.ErrorIs(err, chord.ErrKVLeaseConflict)

	as.NoError(kv.Release(context.Background(), key, token))
}

func TestAcquireExpired(t *testing.T) {
	as := require.New(t)

	kv := testGetKV(t)

	key := make([]byte, 8)
	rand.Read(key)

	// expired 2 seconds ago
	e := kv.writer.WithContext(context.Background()).Create(&LeaseEntry{
		Owner: key,
		Token: uint64(time.Now().Add(time.Duration(-2) * time.Second).UnixNano()),
	})
	as.NoError(e.Error)

	token, err := kv.Acquire(context.Background(), key, time.Second)
	as.NoError(err)

	as.NoError(kv.Release(context.Background(), key, token))
}

func TestRenewValid(t *testing.T) {
	as := assert.New(t)

	kv := testGetKV(t)

	key := make([]byte, 8)
	rand.Read(key)

	token, err := kv.Acquire(context.Background(), key, time.Second)
	as.NoError(err)

	time.Sleep(time.Millisecond * 100)

	n, err := kv.Renew(context.Background(), key, time.Second*2, token)
	as.NoError(err)
	as.NotEqual(token, n)

	// no releasing with the wrong token
	as.ErrorIs(kv.Release(context.Background(), key, token), chord.ErrKVLeaseExpired)
	as.NoError(kv.Release(context.Background(), key, n))
}

func TestRenewExpired(t *testing.T) {
	as := assert.New(t)

	kv := testGetKV(t)

	key := make([]byte, 8)
	rand.Read(key)

	ttl := time.Second

	tk1, err := kv.Acquire(context.Background(), key, ttl)
	as.NoError(err)

	time.Sleep(ttl * 2)

	tk2, err := kv.Acquire(context.Background(), key, ttl)
	as.NoError(err)

	_, err = kv.Renew(context.Background(), key, ttl, tk1)
	as.ErrorIs(err, chord.ErrKVLeaseExpired)

	tk2, err = kv.Renew(context.Background(), key, ttl, tk2)
	as.NoError(err)

	// no releasing with the wrong token
	as.ErrorIs(kv.Release(context.Background(), key, tk1), chord.ErrKVLeaseExpired)
	as.NoError(kv.Release(context.Background(), key, tk2))
}

func TestTTLGuard(t *testing.T) {
	as := assert.New(t)

	kv := testGetKV(t)

	key := make([]byte, 8)
	rand.Read(key)

	ttl := time.Millisecond * 500

	_, err := kv.Acquire(context.Background(), key, ttl)
	as.ErrorIs(err, chord.ErrKVLeaseInvalidTTL)

	ttl = time.Second
	tk, err := kv.Acquire(context.Background(), key, ttl)
	as.NoError(err)

	ttl = time.Millisecond * 500
	_, err = kv.Renew(context.Background(), key, ttl, tk)
	as.ErrorIs(err, chord.ErrKVLeaseInvalidTTL)

	as.NoError(kv.Release(context.Background(), key, tk))
}

// The following tests were collaborated with GPT 4o-mini

func TestLeaseAcquisitionAndRenewal(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	ctx := context.Background()
	leaseID := []byte("lease_test")
	ttl := 1 * time.Second

	// Acquire Lease
	token, err := kv.Acquire(ctx, leaseID, ttl)
	as.NoError(err)

	// Renew Lease before expiration
	newToken, err := kv.Renew(ctx, leaseID, ttl, token)
	as.NoError(err)
	as.Greater(newToken, token) // Ensure token is increasing
}

func TestLeaseExpiration(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	ctx := context.Background()
	leaseID := []byte("lease_test")
	ttl := 1 * time.Second

	// Acquire Lease
	token, err := kv.Acquire(ctx, leaseID, ttl)
	as.NoError(err)

	// Wait for lease to expire
	time.Sleep(ttl + 200*time.Millisecond)

	// Attempt to renew expired lease
	_, err = kv.Renew(ctx, leaseID, ttl, token)
	as.ErrorIs(err, chord.ErrKVLeaseExpired)
}

func TestLeaseRenewalWithParallelLoad(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	ctx := context.Background()
	leaseID := []byte("lease_test")
	ttl := 1 * time.Second

	// Acquire Lease
	token, err := kv.Acquire(ctx, leaseID, ttl)
	as.NoError(err)

	// Start multiple goroutines that try to renew the lease concurrently
	const numWorkers = 50
	errChan := make(chan error, numWorkers)
	tokenChan := make(chan uint64, numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(tokenSnapshot uint64) {
			token, err := kv.Renew(ctx, leaseID, ttl, tokenSnapshot)
			errChan <- err
			tokenChan <- token
		}(token)
	}

	// Collect results
	var successCount int
	for i := 0; i < numWorkers; i++ {
		err := <-errChan
		token := <-tokenChan
		if err == nil && token > 0 {
			successCount++
		} else {
			as.ErrorIs(err, chord.ErrKVLeaseExpired)
		}
	}
	as.Equal(1, successCount, "Only one renewal should succeed")
}

func TestLeaseExpirationUnderLoad(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	ctx := context.Background()
	leaseID := []byte("lease_test")
	ttl := 1 * time.Second

	// Acquire Lease
	token, err := kv.Acquire(ctx, leaseID, ttl)
	as.NoError(err)

	// Start concurrent access while waiting for lease expiration
	const numWorkers = 50
	errChan := make(chan error, numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(tokenSnapshot uint64) {
			time.Sleep(ttl + 200*time.Millisecond) // Wait for expiration
			_, err := kv.Renew(ctx, leaseID, ttl, tokenSnapshot)
			errChan <- err
		}(token)
	}

	// Expect all workers to fail renewal due to expiration
	for i := 0; i < numWorkers; i++ {
		err := <-errChan
		if err == nil {
			t.Errorf("Renewal should have failed, but it succeeded")
		} else {
			as.ErrorIs(err, chord.ErrKVLeaseExpired, "Expired leases should not be renewable")
		}
	}
}

func TestConcurrentLeaseAcquisition(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	ctx := context.Background()
	leaseID := []byte("lease_test")
	ttl := 1 * time.Second

	const numWorkers = 50
	tokenChan := make(chan uint64, numWorkers)
	errChan := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			token, err := kv.Acquire(ctx, leaseID, ttl)
			tokenChan <- token
			errChan <- err
		}()
	}

	var successCount int
	for i := 0; i < numWorkers; i++ {
		err := <-errChan
		token := <-tokenChan
		if err == nil && token > 0 {
			successCount++
		} else {
			as.ErrorIs(err, chord.ErrKVLeaseConflict) // Ensure others fail
		}
	}

	as.Equal(1, successCount, "Only one acquisition should succeed")
}
