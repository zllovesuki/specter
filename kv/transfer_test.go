package kv

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.miragespace.co/specter/kv/aof"
	"go.miragespace.co/specter/kv/memory"
	"go.miragespace.co/specter/kv/sqlite3"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func transferProvider(t *testing.T, name, dir string) (chord.KVProvider, func()) {
	t.Helper()
	switch name {
	case "memory":
		return memory.WithHashFn(chord.Hash), func() {}
	case "aof":
		store, err := aof.New(aof.Config{
			Logger: zap.NewNop(), HasnFn: chord.Hash, DataDir: dir, FlushInterval: time.Second,
		})
		require.NoError(t, err)
		go store.Start()
		t.Cleanup(store.Stop)
		return store, store.Stop
	case "sqlite":
		store, err := sqlite3.New(sqlite3.Config{
			Logger: zap.NewNop(), HashFn: chord.Hash, DataDir: dir,
		})
		require.NoError(t, err)
		t.Cleanup(store.Close)
		return store, store.Close
	default:
		t.Fatalf("unknown provider %q", name)
		return nil, nil
	}
}

// Membership transfers only live keys returned by RangeKeys. Test the actual
// ImportRequest envelope, since direct provider calls preserve Go's nil/empty
// distinction even when the wire format loses it.
func TestSerializedTransferAcrossProviders(t *testing.T) {
	type record struct {
		key    []byte
		value  []byte
		prefix bool
		lease  uint64
	}
	providers := []string{"memory", "aof", "sqlite"}
	for _, source := range providers {
		for _, destination := range providers {
			t.Run(source+"_to_"+destination, func(t *testing.T) {
				ctx := context.Background()
				from, _ := transferProvider(t, source, t.TempDir())
				destinationDir := t.TempDir()
				to, closeDestination := transferProvider(t, destination, destinationDir)
				var records []record
				var liveKeys [][]byte
				var composites []*protocol.KeyComposite
				for _, simple := range []struct {
					name  string
					value []byte
				}{{"absent", nil}, {"empty", []byte{}}, {"populated", []byte("value")}} {
					for flags := range 4 {
						r := record{
							key:   []byte(fmt.Sprintf("%s-%d", simple.name, flags)),
							value: simple.value, prefix: flags&1 != 0,
						}
						if r.value != nil {
							require.NoError(t, from.Put(ctx, r.key, r.value))
							composites = append(composites, &protocol.KeyComposite{Key: r.key, Type: protocol.KeyComposite_SIMPLE})
						}
						if r.prefix {
							require.NoError(t, from.PrefixAppend(ctx, r.key, []byte("child")))
							composites = append(composites, &protocol.KeyComposite{Key: r.key, Type: protocol.KeyComposite_PREFIX})
						}
						if flags&2 != 0 {
							var err error
							r.lease, err = from.Acquire(ctx, r.key, time.Minute)
							require.NoError(t, err)
							composites = append(composites, &protocol.KeyComposite{Key: r.key, Type: protocol.KeyComposite_LEASE})
						}
						records = append(records, r)
						if r.value != nil || r.prefix || r.lease != 0 {
							liveKeys = append(liveKeys, r.key)
						}
					}
				}

				keys, err := from.RangeKeys(ctx, 0, 0)
				require.NoError(t, err)
				require.ElementsMatch(t, liveKeys, keys)
				values, err := from.Export(ctx, keys)
				require.NoError(t, err)
				envelope := &protocol.ImportRequest{Keys: keys, Values: values}
				wire, err := envelope.MarshalVT()
				require.NoError(t, err)
				// Both codecs must agree on explicit empty presence.
				var standard protocol.ImportRequest
				require.NoError(t, proto.Unmarshal(wire, &standard))
				require.True(t, proto.Equal(envelope, &standard))
				var decoded protocol.ImportRequest
				require.NoError(t, decoded.UnmarshalVT(wire))
				require.NoError(t, to.Import(ctx, decoded.Keys, decoded.Values))

				verify := func() {
					t.Helper()
					for _, r := range records {
						value, err := to.Get(ctx, r.key)
						require.NoError(t, err)
						require.Equal(t, r.value, value, "simple value for %s", r.key)
						children, err := to.PrefixList(ctx, r.key)
						require.NoError(t, err)
						if r.prefix {
							require.Equal(t, [][]byte{[]byte("child")}, children)
						} else {
							require.Empty(t, children)
						}
					}
					transferredKeys, err := to.RangeKeys(ctx, 0, 0)
					require.NoError(t, err)
					require.ElementsMatch(t, liveKeys, transferredKeys)
					listed, err := to.ListKeys(ctx, nil)
					require.NoError(t, err)
					require.ElementsMatch(t, composites, listed)
					exported, err := to.Export(ctx, keys)
					require.NoError(t, err)
					require.True(t, proto.Equal(envelope, &protocol.ImportRequest{Keys: keys, Values: exported}))
				}
				verify()
				if destination != "memory" {
					// Exercise SQLite persistence and AOF's nested Import WAL codec.
					closeDestination()
					to, _ = transferProvider(t, destination, destinationDir)
					verify()
				}
				for _, r := range records {
					if r.lease != 0 {
						// The transferred token, not a replacement lease, remains valid.
						require.NoError(t, to.Release(ctx, r.key, r.lease))
					}
				}
			})
		}
	}
}
