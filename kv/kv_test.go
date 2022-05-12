package kv

import (
	"hash/fnv"
	"strconv"
	"strings"
	"testing"

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

	as.LessOrEqual(collisionRing, len(kv.store))

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

	as.LessOrEqual(collisionRing, len(kv.store))

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

	as.LessOrEqual(collisionRing, len(kv.store))

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Delete(ks(keyPrefix, i)))
	}

	for i := 1; i < collisionRing*2; i++ {
		val, err := kv.Get(ks(keyPrefix, i))
		as.Nil(err)
		as.Nil(val)
	}
}
