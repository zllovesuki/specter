package acme

import (
	"bytes"
	"context"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	testLockKey          = "lock-key"
	testLockToken uint64 = 12345678
)

func TestStorageLocker(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)
	kv := new(mocks.VNode)
	defer kv.AssertExpectations(t)

	s, err := NewChordStorage(logger, kv, StorageConfig{
		RetryInterval: time.Millisecond * 100,
		LeaseTTL:      time.Millisecond * 500,
	})
	as.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	acquireCall := kv.On("Acquire", mock.Anything, mock.MatchedBy(func(name []byte) bool {
		return bytes.Equal(name, []byte(kvKeyName(testLockKey)))
	}), mock.MatchedBy(func(ttl time.Duration) bool {
		return ttl == time.Millisecond*500
	})).Return(testLockToken, nil).Once()

	renewCall := kv.On("Renew", mock.Anything, mock.MatchedBy(func(name []byte) bool {
		return bytes.Equal(name, []byte(kvKeyName(testLockKey)))
	}), mock.MatchedBy(func(ttl time.Duration) bool {
		return ttl == time.Millisecond*500
	}), testLockToken).Return(testLockToken, nil).NotBefore(acquireCall)

	kv.On("Release", mock.Anything, mock.MatchedBy(func(name []byte) bool {
		return bytes.Equal(name, []byte(kvKeyName(testLockKey)))
	}), testLockToken).Return(nil).NotBefore(renewCall)

	err = s.Lock(ctx, testLockKey)
	as.NoError(err)

	time.Sleep(time.Second)

	err = s.Unlock(ctx, testLockKey)
	as.NoError(err)
}
