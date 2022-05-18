package kv

import (
	"crypto/rand"
	"hash/fnv"
	"strconv"
	"strings"
	"testing"

	"github.com/zllovesuki/specter/spec/chord"

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

	kv := WithChordHash()

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 10000
	for i := 0; i < num; i++ {
		rand.Read(key)
		rand.Read(value)
		kv.Put(key, value)
	}

	keys, err := kv.LocalKeys(0, 0)
	as.Nil(err)
	as.Len(keys, num)
}

func TestOrderedKeys(t *testing.T) {
	as := assert.New(t)

	kv := WithChordHash()

	key := make([]byte, 64)
	value := make([]byte, 8)

	num := 10000
	for i := 0; i < num; i++ {
		rand.Read(key)
		rand.Read(value)
		kv.Put(key, value)
	}

	keys, err := kv.LocalKeys(0, 0)
	as.Nil(err)

	var prev uint64 = 0
	for _, key := range keys {
		id := chord.Hash(key)
		as.LessOrEqual(prev, id)
		prev = id
	}
}

func TestLocalOperations(t *testing.T) {
	as := assert.New(t)

	kv := WithChordHash()

	num := 32
	length := 8
	keys := make([][]byte, num)
	values := make([][]byte, num)

	for i := range keys {
		keys[i] = make([]byte, length)
		values[i] = make([]byte, length)
		rand.Read(keys[i])
		rand.Read(values[i])
	}

	as.Nil(kv.LocalPuts(keys, values))

	ret, err := kv.LocalGets(keys)
	as.Nil(err)
	as.EqualValues(values, ret)

	as.Nil(kv.LocalDeletes(keys))

	ret, err = kv.LocalGets(keys)
	as.Nil(err)
	as.NotEqualValues(values, ret)
}
