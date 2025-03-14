package sqlite3

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixAppend(t *testing.T) {
	as := assert.New(t)
	kv := testGetKV(t)

	prefix := make([]byte, 8)
	child := make([]byte, 16)
	rand.Read(prefix)
	rand.Read(child)

	as.NoError(kv.PrefixAppend(context.Background(), prefix, child))
	as.Error(kv.PrefixAppend(context.Background(), prefix, child))
}

func TestPrefixList(t *testing.T) {
	as := assert.New(t)
	kv := testGetKV(t)

	numChildren := 32
	prefix := make([]byte, 8)
	rand.Read(prefix)

	children := make([][]byte, numChildren)
	for i := range children {
		children[i] = make([]byte, 16)
		rand.Read(children[i])
		kv.PrefixAppend(context.Background(), prefix, children[i])
	}

	ret, err := kv.PrefixList(context.Background(), prefix)
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

func TestPrefixContains(t *testing.T) {
	as := assert.New(t)
	kv := testGetKV(t)

	prefix := make([]byte, 8)
	child := make([]byte, 16)
	rand.Read(prefix)
	rand.Read(child)

	as.NoError(kv.PrefixAppend(context.Background(), prefix, child))

	b, err := kv.PrefixContains(context.Background(), prefix, child)
	as.NoError(err)
	as.True(b)

	b, err = kv.PrefixContains(context.Background(), prefix, child[1:])
	as.NoError(err)
	as.False(b)
}

func TestPrefixDelete(t *testing.T) {
	as := assert.New(t)
	kv := testGetKV(t)

	numChildren := 32
	prefix := make([]byte, 8)
	rand.Read(prefix)

	children := make([][]byte, numChildren)
	for i := range children {
		children[i] = make([]byte, 16)
		rand.Read(children[i])
		kv.PrefixAppend(context.Background(), prefix, children[i])
	}

	ret, err := kv.PrefixList(context.Background(), prefix)
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
		kv.PrefixRemove(context.Background(), prefix, children[i])
		expectMissing++
	}

	ret, err = kv.PrefixList(context.Background(), prefix)
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
	kv := testGetKV(t)

	key := make([]byte, 8)
	rand.Read(key)

	plainValue := make([]byte, 16)
	rand.Read(plainValue)
	child := make([]byte, 32)
	rand.Read(child)

	as.NoError(kv.Put(context.Background(), key, plainValue))
	as.NoError(kv.PrefixAppend(context.Background(), key, child))

	// deleting the key from plain keyspace should not affect the prefix keyspace
	as.NoError(kv.Delete(context.Background(), key))
	val, err := kv.Get(context.Background(), key)
	as.NoError(err)
	as.Nil(val)

	vals, err := kv.PrefixList(context.Background(), key)
	as.NoError(err)
	as.Len(vals, 1)
	as.EqualValues(child, vals[0])
}
