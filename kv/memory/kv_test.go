package memory

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
)

func TestAllKeys(t *testing.T) {
	as := require.New(t)

	kv := WithHashFn(chord.Hash)

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 10000
	for range num {
		rand.Read(key)
		rand.Read(value)
		kv.Put(context.Background(), key, value)
	}

	keys, err := kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)
	as.Len(keys, num)
}

func TestOrderedKeys(t *testing.T) {
	as := require.New(t)

	kv := WithHashFn(chord.Hash)

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 10000
	for range num {
		rand.Read(key)
		rand.Read(value)
		kv.Put(context.Background(), key, value)
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

	kv := WithHashFn(chord.Hash)

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

	kv := WithHashFn(chord.Hash)

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

	kv2 := WithHashFn(chord.Hash)
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
