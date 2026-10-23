package sqlite3

import (
	"context"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
)

func TestNilAndEmptySimple(t *testing.T) {
	as := require.New(t)
	store := testGetKV(t)
	ctx := context.Background()
	empty := []byte{}

	as.NoError(store.Put(ctx, nil, nil))
	for _, key := range [][]byte{nil, empty} {
		value, err := store.Get(ctx, key)
		as.NoError(err)
		as.Equal(empty, value)
	}

	as.NoError(store.Put(ctx, empty, []byte("updated")))
	value, err := store.Get(ctx, nil)
	as.NoError(err)
	as.Equal([]byte("updated"), value)
	keys, err := store.RangeKeys(ctx, 0, 0)
	as.NoError(err)
	as.Len(keys, 1)
	as.Empty(keys[0])

	as.NoError(store.Delete(ctx, nil))
	value, err = store.Get(ctx, empty)
	as.NoError(err)
	as.Nil(value)
	as.NoError(store.Put(ctx, nil, empty))
	as.NoError(store.Delete(ctx, empty))
	keys, err = store.RangeKeys(ctx, 0, 0)
	as.NoError(err)
	as.Empty(keys)
}

func TestNilAndEmptyPrefix(t *testing.T) {
	as := require.New(t)
	store := testGetKV(t)
	ctx := context.Background()
	empty := []byte{}

	as.NoError(store.PrefixAppend(ctx, nil, nil))
	for _, prefix := range [][]byte{nil, empty} {
		for _, child := range [][]byte{nil, empty} {
			contains, err := store.PrefixContains(ctx, prefix, child)
			as.NoError(err)
			as.True(contains)
		}
		children, err := store.PrefixList(ctx, prefix)
		as.NoError(err)
		as.Len(children, 1)
		as.Empty(children[0])
	}
	as.ErrorIs(store.PrefixAppend(ctx, empty, empty), chord.ErrKVPrefixConflict)
	as.NoError(store.PrefixRemove(ctx, nil, empty))
	contains, err := store.PrefixContains(ctx, empty, nil)
	as.NoError(err)
	as.False(contains)

	as.NoError(store.PrefixAppend(ctx, empty, empty))
	as.NoError(store.PrefixRemove(ctx, empty, nil))
	children, err := store.PrefixList(ctx, nil)
	as.NoError(err)
	as.Empty(children)
	keys, err := store.RangeKeys(ctx, 0, 0)
	as.NoError(err)
	as.Empty(keys)
}

func TestNilAndEmptyLease(t *testing.T) {
	as := require.New(t)
	store := testGetKV(t)
	ctx := context.Background()
	empty := []byte{}

	token, err := store.Acquire(ctx, nil, time.Minute)
	as.NoError(err)
	_, err = store.Acquire(ctx, empty, time.Minute)
	as.ErrorIs(err, chord.ErrKVLeaseConflict)
	token, err = store.Renew(ctx, empty, time.Minute, token)
	as.NoError(err)
	as.NoError(store.Release(ctx, nil, token))

	token, err = store.Acquire(ctx, empty, time.Minute)
	as.NoError(err)
	token, err = store.Renew(ctx, nil, time.Minute, token)
	as.NoError(err)
	as.NoError(store.Release(ctx, empty, token))
	keys, err := store.RangeKeys(ctx, 0, 0)
	as.NoError(err)
	as.Empty(keys)
}

func TestNilAndEmptyTransfer(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  []byte
	}{
		{name: "nil"},
		{name: "empty", key: []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			as := require.New(t)
			store := testGetKV(t)
			ctx := context.Background()
			empty := []byte{}
			value := &protocol.KVTransfer{
				SimpleValue:    empty,
				PrefixChildren: [][]byte{nil, empty},
				LeaseToken:     uint64(time.Now().Add(time.Minute).UnixNano()),
			}
			as.NoError(store.Import(ctx, [][]byte{tc.key}, []*protocol.KVTransfer{value}))
			transferred, err := store.Export(ctx, [][]byte{nil, empty})
			as.NoError(err)
			as.Len(transferred, 2)
			for _, record := range transferred {
				as.Equal(empty, record.SimpleValue)
				as.Len(record.PrefixChildren, 1)
				as.Empty(record.PrefixChildren[0])
				as.Equal(value.LeaseToken, record.LeaseToken)
			}

			var removeKey []byte
			if tc.key == nil {
				removeKey = empty
			}
			as.NoError(store.RemoveKeys(ctx, [][]byte{removeKey}))
			stored, err := store.Get(ctx, tc.key)
			as.NoError(err)
			as.Nil(stored)
			children, err := store.PrefixList(ctx, tc.key)
			as.NoError(err)
			as.Empty(children)
			keys, err := store.RangeKeys(ctx, 0, 0)
			as.NoError(err)
			as.Empty(keys)
			_, err = store.Acquire(ctx, tc.key, time.Minute)
			as.NoError(err)
		})
	}
}
