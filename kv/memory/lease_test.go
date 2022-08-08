package memory

import (
	"crypto/rand"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"

	"github.com/stretchr/testify/assert"
)

func TestAcquireMutualExclusion(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	key := make([]byte, 8)
	rand.Read(key)

	token, err := kv.Acquire(key, time.Second)
	as.NoError(err)

	_, err = kv.Acquire(key, time.Second)
	as.ErrorIs(err, chord.ErrKVLeaseConflict)

	as.NoError(kv.Release(key, token))
}

func TestAcquireExpired(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	key := make([]byte, 8)
	rand.Read(key)

	v, _ := kv.fetchVal(key)
	// expired 2 seconds ago
	v.lease.Store(uint64(time.Now().Add(time.Duration(-2) * time.Second).UnixNano()))

	token, err := kv.Acquire(key, time.Second)
	as.NoError(err)

	as.NoError(kv.Release(key, token))
}

func TestRenewValid(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	key := make([]byte, 8)
	rand.Read(key)

	token, err := kv.Acquire(key, time.Second)
	as.NoError(err)

	time.Sleep(time.Millisecond * 100)

	n, err := kv.Renew(key, time.Second*2, token)
	as.NoError(err)
	as.NotEqual(token, n)

	// no releasing with the wrong token
	as.ErrorIs(kv.Release(key, token), chord.ErrKVLeaseExpired)
	as.NoError(kv.Release(key, n))
}

func TestRenewExpired(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	key := make([]byte, 8)
	rand.Read(key)

	ttl := time.Second

	tk1, err := kv.Acquire(key, ttl)
	as.NoError(err)

	time.Sleep(ttl * 2)

	tk2, err := kv.Acquire(key, ttl)
	as.NoError(err)

	_, err = kv.Renew(key, ttl, tk1)
	as.ErrorIs(err, chord.ErrKVLeaseExpired)

	tk2, err = kv.Renew(key, ttl, tk2)
	as.NoError(err)

	// no releasing with the wrong token
	as.ErrorIs(kv.Release(key, tk1), chord.ErrKVLeaseExpired)
	as.NoError(kv.Release(key, tk2))
}

func TestTTLGuard(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	key := make([]byte, 8)
	rand.Read(key)

	ttl := time.Millisecond * 500

	_, err := kv.Acquire(key, ttl)
	as.ErrorIs(err, chord.ErrKVLeaseInvalidTTL)

	ttl = time.Second
	tk, err := kv.Acquire(key, ttl)
	as.NoError(err)

	ttl = time.Millisecond * 500
	_, err = kv.Renew(key, ttl, tk)
	as.ErrorIs(err, chord.ErrKVLeaseInvalidTTL)

	as.NoError(kv.Release(key, tk))
}
