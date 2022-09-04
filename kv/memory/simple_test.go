package memory

import (
	"context"
	"crypto/rand"
	"hash/fnv"
	"strconv"
	"strings"
	"testing"

	"kon.nect.sh/specter/spec/chord"

	"github.com/stretchr/testify/assert"
)

const (
	collisionRing = 8
	keyPrefix     = "k"
	valPrefix     = "val"
)

func collisionHash(s []byte) uint64 {
	h := fnv.New64a()
	h.Write(s)
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
		as.Nil(kv.Put(context.Background(), ks(keyPrefix, i), ks(valPrefix, i)))
	}

	as.LessOrEqual(collisionRing, kv.s.Len())

	for i := 1; i < collisionRing*2; i++ {
		val, err := kv.Get(context.Background(), ks(keyPrefix, i))
		as.Nil(err)
		as.Equal(ks(valPrefix, i), val)
	}
}

func TestCollisionNil(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(collisionHash)

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Put(context.Background(), ks(keyPrefix, i), ks(valPrefix, i)))
	}

	as.LessOrEqual(collisionRing, kv.s.Len())

	for i := collisionRing * 2; i < collisionRing*4; i++ {
		val, err := kv.Get(context.Background(), ks(keyPrefix, i))
		as.Nil(err)
		as.Nil(val)
	}
}

func TestCollisionDelete(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(collisionHash)

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Put(context.Background(), ks(keyPrefix, i), ks(valPrefix, i)))
	}

	as.LessOrEqual(collisionRing, kv.s.Len())

	for i := 1; i < collisionRing*2; i++ {
		as.Nil(kv.Delete(context.Background(), ks(keyPrefix, i)))
	}

	for i := 1; i < collisionRing*2; i++ {
		val, err := kv.Get(context.Background(), ks(keyPrefix, i))
		as.Nil(err)
		as.Nil(val)
	}
}

func TestEmpty(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.Hash)

	key := make([]byte, 6)
	rand.Read(key)

	val, err := kv.Get(context.Background(), key)
	as.NoError(err)
	as.Nil(val)

	err = kv.Put(context.Background(), key, []byte("v"))
	as.NoError(err)

	val, err = kv.Get(context.Background(), key)
	as.NoError(err)
	as.NotNil(val)

	err = kv.Delete(context.Background(), key)
	as.NoError(err)

	val, err = kv.Get(context.Background(), key)
	as.NoError(err)
	as.Nil(val)

	keys := kv.RangeKeys(0, 0)
	exp := kv.Export(keys)

	kv2 := WithHashFn(chord.Hash)
	as.NoError(kv2.Import(context.Background(), keys, exp))

	val, err = kv2.Get(context.Background(), key)
	as.NoError(err)
	as.Nil(val)

	err = kv2.Put(context.Background(), key, []byte{})
	as.NoError(err)

	val, err = kv2.Get(context.Background(), key)
	as.NoError(err)
	as.NotNil(val)
}
