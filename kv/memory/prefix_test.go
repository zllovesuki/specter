package memory

import (
	"bytes"
	"crypto/rand"
	"testing"

	"kon.nect.sh/specter/spec/chord"

	"github.com/stretchr/testify/assert"
)

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

func TestPrefixContains(t *testing.T) {
	as := assert.New(t)

	kv := WithHashFn(chord.HashString)

	prefix := make([]byte, 8)
	child := make([]byte, 16)
	rand.Read(prefix)
	rand.Read(child)

	as.NoError(kv.PrefixAppend(prefix, child))

	b, err := kv.PrefixContains(prefix, child)
	as.NoError(err)
	as.True(b)

	b, err = kv.PrefixContains(prefix, child[1:])
	as.NoError(err)
	as.False(b)
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
