package aof

import (
	"crypto/rand"
	"io/fs"
	"os"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStartStop(t *testing.T) {
	as := require.New(t)
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	dir, err := os.MkdirTemp("", "aof")
	as.NoError(err)
	defer os.RemoveAll(dir)

	cfg := Config{
		Logger:        logger,
		HasnFn:        chord.Hash,
		DataDir:       dir,
		FlushInterval: time.Millisecond * 500,
	}

	kv, err := New(cfg)
	as.NoError(err)
	go kv.Start()

	key := make([]byte, 8)
	value := make([]byte, 16)

	rand.Read(key)
	rand.Read(value)

	err = kv.Put(key, value)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg)
	as.NoError(err)
	go kv.Start()

	val, err := kv.Get(key)
	as.NoError(err)
	as.Equal(value, val)

	rand.Read(key)
	rand.Read(value)

	err = kv.Put(key, value)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg)
	as.NoError(err)

	val, err = kv.Get(key)
	as.NoError(err)
	as.Equal(value, val)
}

func TestEverything(t *testing.T) {
	as := require.New(t)
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	dir, err := os.MkdirTemp("", "aof")
	as.NoError(err)
	defer os.RemoveAll(dir)

	cfg := Config{
		Logger:        logger,
		HasnFn:        chord.Hash,
		DataDir:       dir,
		FlushInterval: time.Millisecond * 500,
	}

	kv, err := New(cfg)
	as.NoError(err)
	go kv.Start()

	key := make([]byte, 8)
	value := make([]byte, 16)

	rand.Read(key)
	rand.Read(value)

	err = kv.Put(key, value)
	as.NoError(err)

	err = kv.Put(value, key)
	as.NoError(err)

	err = kv.Delete(value)
	as.NoError(err)

	err = kv.PrefixAppend(key, value)
	as.NoError(err)

	err = kv.PrefixAppend(value, key)
	as.NoError(err)

	err = kv.PrefixRemove(value, key)
	as.NoError(err)

	time.Sleep(cfg.FlushInterval * 2)

	keys := kv.RangeKeys(0, 0)
	snapshot1 := kv.Export(keys)

	kv.Stop()

	// mutations after close should error
	err = kv.Put(key, value)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.Delete(key)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.PrefixAppend(key, value)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.PrefixRemove(key, value)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.Import(keys, snapshot1)
	as.ErrorIs(err, fs.ErrClosed)
	kv.RemoveKeys(keys) // no-op

	kv.Stop() // no-op

	kv, err = New(cfg)
	as.NoError(err)
	go kv.Start()

	val, err := kv.Get(key)
	as.NoError(err)
	as.Equal(value, val)

	ll, err := kv.PrefixList(key)
	as.NoError(err)
	as.Len(ll, 1)
	as.EqualValues(value, ll[0])

	has, err := kv.PrefixContains(key, value)
	as.NoError(err)
	as.True(has)

	has, err = kv.PrefixContains(value, key)
	as.NoError(err)
	as.False(has)

	val, err = kv.Get(value)
	as.NoError(err)
	as.Nil(val)

	keys = kv.RangeKeys(0, 0)
	snapshot2 := kv.Export(keys)

	as.EqualValues(snapshot1, snapshot2)

	kv.Stop()
}

func TestImportAndRemoveKeys(t *testing.T) {
	as := require.New(t)
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	dir1, err := os.MkdirTemp("", "aof")
	as.NoError(err)
	defer os.RemoveAll(dir1)

	cfg1 := Config{
		Logger:        logger,
		HasnFn:        chord.Hash,
		DataDir:       dir1,
		FlushInterval: time.Millisecond * 500,
	}

	kv, err := New(cfg1)
	as.NoError(err)
	go kv.Start()

	keys := make([][]byte, 16)
	values := make([][]byte, 16)
	for i := range keys {
		keys[i] = make([]byte, 8)
		values[i] = make([]byte, 16)
		rand.Read(keys[i])
		rand.Read(values[i])
	}

	for i := range keys {
		err := kv.Put(keys[i], values[i])
		as.NoError(err)
	}

	expKeys := kv.RangeKeys(0, 0)
	snapshot := kv.Export(expKeys)

	kv.Stop()

	dir2, err := os.MkdirTemp("", "aof")
	as.NoError(err)
	defer os.RemoveAll(dir2)

	cfg2 := Config{
		Logger:        logger,
		HasnFn:        chord.Hash,
		DataDir:       dir2,
		FlushInterval: time.Millisecond * 500,
	}

	kv, err = New(cfg2)
	as.NoError(err)
	go kv.Start()

	for i := range keys {
		val, err := kv.Get(keys[i])
		as.NoError(err)
		as.Nil(val)
	}

	err = kv.Import(expKeys, snapshot)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg2)
	as.NoError(err)
	go kv.Start()

	for i := range keys {
		val, err := kv.Get(keys[i])
		as.NoError(err)
		as.EqualValues(values[i], val)
	}

	kv.RemoveKeys(expKeys)

	kv.Stop()

	kv, err = New(cfg2)
	as.NoError(err)
	go kv.Start()

	for i := range keys {
		val, err := kv.Get(keys[i])
		as.NoError(err)
		as.Nil(val)
	}

	kv.Stop()
}

// Lease KV operations are volatile
func TestVolatile(t *testing.T) {
	as := require.New(t)
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	dir1, err := os.MkdirTemp("", "aof")
	as.NoError(err)
	defer os.RemoveAll(dir1)

	cfg1 := Config{
		Logger:        logger,
		HasnFn:        chord.Hash,
		DataDir:       dir1,
		FlushInterval: time.Millisecond * 500,
	}

	kv, err := New(cfg1)
	as.NoError(err)
	go kv.Start()

	token, err := kv.Acquire([]byte("lease"), time.Second)
	as.NoError(err)

	token, err = kv.Renew([]byte("lease"), time.Second, token)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg1)
	as.NoError(err)
	go kv.Start()

	err = kv.Release([]byte("lease"), token)
	as.ErrorIs(err, chord.ErrKVLeaseExpired)

	kv.Stop()
}

func TestConflictRollback(t *testing.T) {
	as := require.New(t)
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	dir1, err := os.MkdirTemp("", "aof")
	as.NoError(err)
	defer os.RemoveAll(dir1)

	cfg1 := Config{
		Logger:        logger,
		HasnFn:        chord.Hash,
		DataDir:       dir1,
		FlushInterval: time.Millisecond * 500,
	}

	kv, err := New(cfg1)
	as.NoError(err)
	go kv.Start()

	err = kv.PrefixAppend([]byte("prefix"), []byte("child"))
	as.NoError(err)
	err = kv.PrefixAppend([]byte("prefix"), []byte("child"))
	as.ErrorIs(err, chord.ErrKVPrefixConflict)

	err = kv.PrefixAppend([]byte("prefix"), []byte("grandchild"))
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg1)
	as.NoError(err)
	go kv.Start()

	has, err := kv.PrefixContains([]byte("prefix"), []byte("child"))
	as.NoError(err)
	as.True(has)

	has, err = kv.PrefixContains([]byte("prefix"), []byte("grandchild"))
	as.NoError(err)
	as.True(has)

	kv.Stop()
}
