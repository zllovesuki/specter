package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
