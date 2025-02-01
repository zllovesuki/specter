package sqlite3

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmpty(t *testing.T) {
	as := assert.New(t)
	kv := testGetKV(t)

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

	keys, err := kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)
	exp, err := kv.Export(context.Background(), keys)
	as.NoError(err)

	kv2 := testGetKV(t)
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
