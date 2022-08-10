package memory

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"kon.nect.sh/specter/spec/chord"
)

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

func TestEmpty(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	key := make([]byte, 6)
	rand.Read(key)

	val, err := kv.Get(key)
	as.NoError(err)
	as.Nil(val)

	err = kv.Put(key, []byte("v"))
	as.NoError(err)

	val, err = kv.Get(key)
	as.NoError(err)
	as.NotNil(val)

	err = kv.Delete(key)
	as.NoError(err)

	val, err = kv.Get(key)
	as.NoError(err)
	as.Nil(val)

	keys := kv.RangeKeys(0, 0)
	exp := kv.Export(keys)

	kv2 := WithHashFn(chord.HashString)
	as.NoError(kv2.Import(keys, exp))

	val, err = kv2.Get(key)
	as.NoError(err)
	as.Nil(val)

	err = kv2.Put(key, []byte{})
	as.NoError(err)

	val, err = kv2.Get(key)
	as.NoError(err)
	as.NotNil(val)
}
