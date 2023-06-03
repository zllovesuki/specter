package chord

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/protocol"
)

type SimpleKV interface {
	// Put will store the value to a node in the Chord network responsible for the given key.
	// If the key did not exist, a new entry will be added.
	// If the key already exist, the value will be overwrriten.
	// If the key was concurrently modified by another request, ErrKVSimpleConflict error is returned.
	Put(ctx context.Context, key, value []byte) error
	// Get will fetch the value from a node in the Chord network.
	Get(ctx context.Context, key []byte) (value []byte, err error)
	// Delete will hard delete the key from the Chord network.
	// If the key was concurrently modified by another request, ErrKVSimpleConflict error is returned.
	// Note that Put/Get methods can share the same keyspace as Prefix methods, and
	// Delete will not remove the Prefix children.
	Delete(ctx context.Context, key []byte) error
}

type PrefixKV interface {
	// PrefixAppend appends the child under the prefix. This is useful for tracking
	// hierarchical structure such as directories.
	// If the child already exist, ErrKVPrefixConflict error is returned.
	// Note that Prefix methods can share the same keyspace as Put/Get, and
	// Delete will not remove the Prefix children.
	PrefixAppend(ctx context.Context, prefix []byte, child []byte) error
	// PrefixList returns the children under the prefix.
	PrefixList(ctx context.Context, prefix []byte) (children [][]byte, err error)
	// PrefixContains checks if the child is in the prefix children
	PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error)
	// PrefixRemove removes the matching child under the prefix.
	// If the child did not exist, this is an no-op
	PrefixRemove(ctx context.Context, prefix []byte, child []byte) error
}

type LeaseKV interface {
	// Acquire obtains a lease with given time-to-live, and returns a token for
	// later renewal and release.
	// On conflicting lease acquisition, ErrKVLeaseConflict error is returned.
	// If ttl is less than a second, ErrKVLeaseInvalidTTL error is returned.
	// Not to be confused with memory ordering acquire/release semantics.
	Acquire(ctx context.Context, lease []byte, ttl time.Duration) (token uint64, err error)
	// Renew extends the lease with given time-to-live, given that prevToken
	// is still valid. If the renewal occurs after a previous acquire
	// has expired and a different lease was acquired, ErrKVLeaseExpired error is returned.
	// If ttl is less than a second, ErrKVLeaseInvalidTTL error is returned.
	Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error)
	// Release relinquish the lease held previously by the given token.
	// If the lease holder has changed, ErrKVLeaseExpired error is returned.
	// Not to be confused with memory ordering acquire/release semantics.
	Release(ctx context.Context, lease []byte, token uint64) error
}

type KV interface {
	SimpleKV
	PrefixKV
	LeaseKV
	// Import is used when a node is transferring its KV to a remote node.
	// Used when a new node joins or a node leaves gracefully
	Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error
	// ListKeys is used for iterating through the network to find keys with the given prefix.
	ListKeys(ctx context.Context, prefix []byte, typ protocol.ListKeysRequest_Type) ([][]byte, error)
}

type KVProvider interface {
	KV
	// Export is used when a Local node is retrieving relevant keys to transfer.
	// Only used locally, not used for RPC
	Export(keys [][]byte) []*protocol.KVTransfer
	// RangeKeys retrieve actual byte values of the keys, given the [low, high]
	// range of key hashes.
	// Only used locally, not used for RPC
	RangeKeys(low, high uint64) [][]byte
	// RemoveKeys hard delete keys from local node.
	// Only used locally, not used for RPC
	RemoveKeys(keys [][]byte)
}
