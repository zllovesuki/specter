package aof

import (
	"context"
	"crypto/rand"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestStartStop(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

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

	err = kv.Put(context.Background(), key, value)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg)
	as.NoError(err)
	go kv.Start()

	val, err := kv.Get(context.Background(), key)
	as.NoError(err)
	as.Equal(value, val)

	rand.Read(key)
	rand.Read(value)

	err = kv.Put(context.Background(), key, value)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg)
	as.NoError(err)

	val, err = kv.Get(context.Background(), key)
	as.NoError(err)
	as.Equal(value, val)
}

func TestEverything(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

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

	err = kv.Put(context.Background(), key, value)
	as.NoError(err)

	err = kv.Put(context.Background(), value, key)
	as.NoError(err)

	err = kv.Delete(context.Background(), value)
	as.NoError(err)

	err = kv.PrefixAppend(context.Background(), key, value)
	as.NoError(err)

	err = kv.PrefixAppend(context.Background(), value, key)
	as.NoError(err)

	err = kv.PrefixRemove(context.Background(), value, key)
	as.NoError(err)

	time.Sleep(cfg.FlushInterval * 2)

	keys, err := kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)
	snapshot1, err := kv.Export(context.Background(), keys)
	as.NoError(err)

	kv.Stop()

	// mutations after close should error
	err = kv.Put(context.Background(), key, value)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.Delete(context.Background(), key)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.PrefixAppend(context.Background(), key, value)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.PrefixRemove(context.Background(), key, value)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.Import(context.Background(), keys, snapshot1)
	as.ErrorIs(err, fs.ErrClosed)
	err = kv.RemoveKeys(context.Background(), keys) // no-op
	as.NoError(err)

	kv.Stop() // no-op

	kv, err = New(cfg)
	as.NoError(err)
	go kv.Start()

	val, err := kv.Get(context.Background(), key)
	as.NoError(err)
	as.Equal(value, val)

	ll, err := kv.PrefixList(context.Background(), key)
	as.NoError(err)
	as.Len(ll, 1)
	as.EqualValues(value, ll[0])

	has, err := kv.PrefixContains(context.Background(), key, value)
	as.NoError(err)
	as.True(has)

	has, err = kv.PrefixContains(context.Background(), value, key)
	as.NoError(err)
	as.False(has)

	val, err = kv.Get(context.Background(), value)
	as.NoError(err)
	as.Nil(val)

	keys, err = kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)
	snapshot2, err := kv.Export(context.Background(), keys)
	as.NoError(err)

	as.EqualValues(snapshot1, snapshot2)

	kv.Stop()
}

func TestImportAndRemoveKeys(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

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
		err := kv.Put(context.Background(), keys[i], values[i])
		as.NoError(err)
	}

	expKeys, err := kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)
	snapshot, err := kv.Export(context.Background(), expKeys)
	as.NoError(err)

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
		val, err := kv.Get(context.Background(), keys[i])
		as.NoError(err)
		as.Nil(val)
	}

	err = kv.Import(context.Background(), expKeys, snapshot)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg2)
	as.NoError(err)
	go kv.Start()

	for i := range keys {
		val, err := kv.Get(context.Background(), keys[i])
		as.NoError(err)
		as.EqualValues(values[i], val)
	}

	err = kv.RemoveKeys(context.Background(), expKeys)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg2)
	as.NoError(err)
	go kv.Start()

	for i := range keys {
		val, err := kv.Get(context.Background(), keys[i])
		as.NoError(err)
		as.Nil(val)
	}

	kv.Stop()
}

// Lease KV operations are volatile
func TestVolatile(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

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

	token, err := kv.Acquire(context.Background(), []byte("lease"), time.Second)
	as.NoError(err)

	token, err = kv.Renew(context.Background(), []byte("lease"), time.Second, token)
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg1)
	as.NoError(err)
	go kv.Start()

	err = kv.Release(context.Background(), []byte("lease"), token)
	as.ErrorIs(err, chord.ErrKVLeaseExpired)

	kv.Stop()
}

func TestConflictRollback(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

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

	err = kv.PrefixAppend(context.Background(), []byte("prefix"), []byte("child"))
	as.NoError(err)
	err = kv.PrefixAppend(context.Background(), []byte("prefix"), []byte("child"))
	as.ErrorIs(err, chord.ErrKVPrefixConflict)

	err = kv.PrefixAppend(context.Background(), []byte("prefix"), []byte("grandchild"))
	as.NoError(err)

	kv.Stop()

	kv, err = New(cfg1)
	as.NoError(err)
	go kv.Start()

	has, err := kv.PrefixContains(context.Background(), []byte("prefix"), []byte("child"))
	as.NoError(err)
	as.True(has)

	has, err = kv.PrefixContains(context.Background(), []byte("prefix"), []byte("grandchild"))
	as.NoError(err)
	as.True(has)

	kv.Stop()
}

func TestCorruptedLog(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

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

	err = kv.Put(context.Background(), []byte("key"), []byte("value"))
	as.NoError(err)

	kv.Stop()

	files, err := os.ReadDir(logPath(dir1))
	as.NoError(err)

	logFile := files[0]
	f, err := os.OpenFile(filepath.Join(logPath(dir1), logFile.Name()), os.O_RDWR, logFile.Type())
	as.NoError(err)

	info, err := logFile.Info()
	as.NoError(err)

	buf := make([]byte, info.Size())
	f.Read(buf)
	t.Log(buf)
	buf[10] = 0
	buf[15] = 0
	t.Log(buf)
	f.Seek(0, io.SeekStart)
	f.Write(buf)
	f.Sync()

	f.Close()

	_, err = New(cfg1)
	as.Error(err)
}
