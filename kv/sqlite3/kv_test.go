package sqlite3

import (
	"context"
	"crypto/rand"
	"os"
	"sync"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
)

func testGetKV(t *testing.T) *SqliteKV {
	t.Helper()

	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	dir, err := os.MkdirTemp("", "sql")
	as.NoError(err)

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	cfg := Config{
		Logger:  logger,
		HasnFn:  chord.Hash,
		DataDir: dir,
	}

	kv, err := New(cfg)
	as.NoError(err)

	return kv
}

func TestAllKeys(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 1000
	for i := 0; i < num; i++ {
		rand.Read(key)
		rand.Read(value)
		err := kv.Put(context.Background(), key, value)
		as.NoError(err)
	}

	keys, err := kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)
	as.Len(keys, num)
}

func TestOrderedKeys(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 1000
	for i := 0; i < num; i++ {
		rand.Read(key)
		rand.Read(value)
		err := kv.Put(context.Background(), key, value)
		as.NoError(err)
	}

	keys, err := kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)

	var prev uint64 = 0
	for _, key := range keys {
		id := chord.Hash(key)
		as.LessOrEqual(prev, id)
		prev = id
	}
}

func TestLocalOperations(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	num := 32
	length := 8
	keys := make([][]byte, num)
	values := make([]*protocol.KVTransfer, num)

	for i := range keys {
		keys[i] = make([]byte, length)
		values[i] = &protocol.KVTransfer{
			SimpleValue:    make([]byte, length),
			PrefixChildren: make([][]byte, 0),
		}
		rand.Read(keys[i])
		rand.Read(values[i].SimpleValue)
	}

	as.Nil(kv.Import(context.Background(), keys, values))

	ret, err := kv.Export(context.Background(), keys)
	as.NoError(err)
	as.EqualValues(values, ret)

	err = kv.RemoveKeys(context.Background(), keys)
	as.NoError(err)

	ret, err = kv.Export(context.Background(), keys)
	as.NoError(err)
	as.NotEqualValues(values, ret)
}

func TestComplexImportExport(t *testing.T) {
	as := require.New(t)
	kv := testGetKV(t)

	key := make([]byte, 8)
	rand.Read(key)

	plainValue := make([]byte, 16)
	rand.Read(plainValue)
	child := make([]byte, 32)
	rand.Read(child)

	as.NoError(kv.Put(context.Background(), key, plainValue))
	as.NoError(kv.PrefixAppend(context.Background(), key, child))
	tk, err := kv.Acquire(context.Background(), key, time.Second)
	as.NoError(err)

	val, err := kv.Get(context.Background(), key)
	as.NoError(err)
	as.EqualValues(plainValue, val)

	vals, err := kv.PrefixList(context.Background(), key)
	as.NoError(err)
	as.Len(vals, 1)
	as.EqualValues(child, vals[0])

	keys, err := kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)
	exp, err := kv.Export(context.Background(), keys)
	as.NoError(err)

	kv2 := testGetKV(t)
	as.NoError(kv2.Import(context.Background(), keys, exp))

	val, err = kv2.Get(context.Background(), key)
	as.NoError(err)
	as.EqualValues(plainValue, val)

	vals, err = kv2.PrefixList(context.Background(), key)
	as.NoError(err)
	as.Len(vals, 1)
	as.EqualValues(child, vals[0])

	tk2, err := kv2.Renew(context.Background(), key, time.Second, tk)
	as.NoError(err)
	as.NoError(kv2.Release(context.Background(), key, tk2))
}

// The following test was collaborated with GPT 4o-mini
func TestConcurrentOps(t *testing.T) {
	kv := testGetKV(t)
	as := require.New(t)

	keys := make([][]byte, 8)
	for i := range keys {
		keys[i] = make([]byte, 8)
		rand.Read(keys[i])
	}

	var wg sync.WaitGroup
	const numGoroutines = 50

	testOperations := func(ctx context.Context, key []byte, id int) {
		defer wg.Done()

		err := kv.Put(ctx, key, []byte("test_data"))
		as.NoError(err)

		prefixValue := "prefix_value_" + string(rune(id))
		err = kv.PrefixAppend(ctx, key, []byte(prefixValue))
		as.NoError(err)

		token, err := kv.Acquire(ctx, key, time.Second*5)
		if err == nil {
			token, err = kv.Renew(ctx, key, time.Second*5, token)
			as.NoError(err)

			err = kv.Release(ctx, key, token)
			as.NoError(err)
		}

		_, err = kv.Get(ctx, key)
		as.NoError(err)

		_, err = kv.PrefixList(ctx, key)
		as.NoError(err)

		err = kv.PrefixAppend(ctx, key, []byte(prefixValue))
		as.Error(err)

		err = kv.Delete(ctx, key)
		as.NoError(err)

		err = kv.PrefixRemove(ctx, key, []byte(prefixValue))
		as.NoError(err)
	}

	ctx := context.Background()
	for i := 0; i < numGoroutines; i++ {
		for _, key := range keys {
			wg.Add(1)
			go testOperations(ctx, key, i)
		}
	}

	wg.Wait()

	for _, key := range keys {
		tracker := KeyTracker{
			Key: key,
		}
		resp := kv.reader.Where("key = ?", key).Take(&tracker)
		as.ErrorIs(resp.Error, gorm.ErrRecordNotFound)
	}

}
