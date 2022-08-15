package memory

import (
	"crypto/rand"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/assert"
)

func TestAllKeys(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.Hash)

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 10000
	for i := 0; i < num; i++ {
		rand.Read(key)
		rand.Read(value)
		kv.Put(key, value)
	}

	keys := kv.RangeKeys(0, 0)
	as.Len(keys, num)
}

func TestOrderedKeys(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.Hash)

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 10000
	for i := 0; i < num; i++ {
		rand.Read(key)
		rand.Read(value)
		kv.Put(key, value)
	}

	keys := kv.RangeKeys(0, 0)

	var prev uint64 = 0
	for _, key := range keys {
		id := chord.Hash(key)
		as.LessOrEqual(prev, id)
		prev = id
	}
}

func TestLocalOperations(t *testing.T) {
	as := assert.New(t)

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

	as.Nil(kv.Import(keys, values))

	ret := kv.Export(keys)
	as.EqualValues(values, ret)

	kv.RemoveKeys(keys)

	ret = kv.Export(keys)
	as.NotEqualValues(values, ret)
}

func TestComplexImportExport(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.Hash)

	key := make([]byte, 8)
	rand.Read(key)

	plainValue := make([]byte, 16)
	rand.Read(plainValue)
	child := make([]byte, 32)
	rand.Read(child)

	as.NoError(kv.Put(key, plainValue))
	as.NoError(kv.PrefixAppend(key, child))
	tk, err := kv.Acquire(key, time.Second)
	as.NoError(err)

	val, err := kv.Get(key)
	as.NoError(err)
	as.EqualValues(plainValue, val)

	vals, err := kv.PrefixList(key)
	as.NoError(err)
	as.Len(vals, 1)
	as.EqualValues(child, vals[0])

	keys := kv.RangeKeys(0, 0)
	exp := kv.Export(keys)

	kv2 := WithHashFn(chord.Hash)
	as.NoError(kv2.Import(keys, exp))

	val, err = kv2.Get(key)
	as.NoError(err)
	as.EqualValues(plainValue, val)

	vals, err = kv2.PrefixList(key)
	as.NoError(err)
	as.Len(vals, 1)
	as.EqualValues(child, vals[0])

	tk2, err := kv2.Renew(key, time.Second, tk)
	as.NoError(err)
	as.NoError(kv2.Release(key, tk2))
}
