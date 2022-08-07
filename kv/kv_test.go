package kv

import (
	"bytes"
	"crypto/rand"
	"hash/fnv"
	"strconv"
	"strings"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/assert"
)

const (
	collisionRing = 8
	keyPrefix     = "k"
	valPrefix     = "val"
)

func collisionHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64() % collisionRing
}

func ks(p string, i int) []byte {
	var sb strings.Builder
	sb.WriteString(p)
	sb.WriteString("/")
	sb.WriteString(strconv.FormatInt(int64(i), 10))
	return []byte(sb.String())
}

func TestCollisionPutGet(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(collisionHash)

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Put(ks(keyPrefix, i), ks(valPrefix, i)))
	}

	as.LessOrEqual(collisionRing, kv.s.Len())

	for i := 1; i < collisionRing*2; i++ {
		val, err := kv.Get(ks(keyPrefix, i))
		as.Nil(err)
		as.Equal(ks(valPrefix, i), val)
	}
}

func TestCollisionNil(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(collisionHash)

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Put(ks(keyPrefix, i), ks(valPrefix, i)))
	}

	as.LessOrEqual(collisionRing, kv.s.Len())

	for i := collisionRing * 2; i < collisionRing*4; i++ {
		val, err := kv.Get(ks(keyPrefix, i))
		as.Nil(err)
		as.Nil(val)
	}
}

func TestCollisionDelete(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(collisionHash)

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Put(ks(keyPrefix, i), ks(valPrefix, i)))
	}

	as.LessOrEqual(collisionRing, kv.s.Len())

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Delete(ks(keyPrefix, i)))
	}

	for i := 1; i < collisionRing*2; i++ {
		val, err := kv.Get(ks(keyPrefix, i))
		as.Nil(err)
		as.Nil(val)
	}
}

func TestAllKeys(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

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

	kv := WithHashFn(chord.HashString)

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

	kv := WithHashFn(chord.HashString)

	num := 32
	length := 8
	keys := make([][]byte, num)
	values := make([]*protocol.KVTransfer, num)

	for i := range keys {
		keys[i] = make([]byte, length)
		values[i] = &protocol.KVTransfer{
			PlainValue:     make([]byte, length),
			PrefixChildren: make([][]byte, 0),
		}
		rand.Read(keys[i])
		rand.Read(values[i].PlainValue)
	}

	as.Nil(kv.Import(keys, values))

	ret := kv.Export(keys)
	as.EqualValues(values, ret)

	kv.RemoveKeys(keys)

	ret = kv.Export(keys)
	as.NotEqualValues(values, ret)
}

func TestPrefixAppend(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	prefix := make([]byte, 8)
	child := make([]byte, 16)
	rand.Read(prefix)
	rand.Read(child)

	as.NoError(kv.PrefixAppend(prefix, child))
	as.Error(kv.PrefixAppend(prefix, child))
}

func TestPrefixList(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	numChildren := 32
	prefix := make([]byte, 8)
	children := make([][]byte, numChildren)
	for i := range children {
		children[i] = make([]byte, 16)
		rand.Read(children[i])
		kv.PrefixAppend(prefix, children[i])
	}

	ret, err := kv.PrefixList(prefix)
	as.NoError(err)
	for _, child := range ret {
		as.Greater(len(child), 0)
	}

	found := 0
	for _, needle := range children {
		for _, haystack := range ret {
			if bytes.Equal(haystack, needle) {
				found++
			}
		}
	}

	as.Equal(numChildren, found, "missing from prefix list")
}

func TestPrefixDelete(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	numChildren := 32
	prefix := make([]byte, 8)
	children := make([][]byte, numChildren)
	for i := range children {
		children[i] = make([]byte, 16)
		rand.Read(children[i])
		kv.PrefixAppend(prefix, children[i])
	}

	ret, err := kv.PrefixList(prefix)
	as.NoError(err)

	found := 0
	for _, needle := range children {
		for _, haystack := range ret {
			if bytes.Equal(haystack, needle) {
				found++
			}
		}
	}

	as.Equal(numChildren, found, "missing child from prefix list")

	expectMissing := 0
	for i := 0; i < numChildren; i += 2 {
		kv.PrefixRemove(prefix, children[i])
		expectMissing++
	}

	ret, err = kv.PrefixList(prefix)
	as.NoError(err)

	found = 0
	for _, needle := range children {
		for _, haystack := range ret {
			if bytes.Equal(haystack, needle) {
				found++
			}
		}
	}

	as.Equal(numChildren-expectMissing, found, "found deleted child")
}

func TestSharedKeyspace(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	key := make([]byte, 8)
	rand.Read(key)

	plainValue := make([]byte, 16)
	rand.Read(plainValue)
	child := make([]byte, 32)
	rand.Read(child)

	as.NoError(kv.Put(key, plainValue))
	as.NoError(kv.PrefixAppend(key, child))

	// deleting the key from plain keyspace should not affect the prefix keyspace
	as.NoError(kv.Delete(key))
	val, err := kv.Get(key)
	as.NoError(err)
	as.Nil(val)

	vals, err := kv.PrefixList(key)
	as.NoError(err)
	as.Len(vals, 1)
	as.EqualValues(child, vals[0])
}

func TestComplexImportExport(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

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

	kv2 := WithHashFn(chord.HashString)
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
