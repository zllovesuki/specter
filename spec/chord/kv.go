package chord

import (
	"time"

	"kon.nect.sh/specter/spec/protocol"
)

type KV interface {
	// Put will store the value to a node in the Chord network responsible for the given key.
	// If the key did not exist, a new entry will be added.
	// If the key already exist, the value will be overwrriten
	Put(key, value []byte) error
	// Get will fetch the value from a node in the Chord network
	// TODO: separate nil vs not found
	Get(key []byte) (value []byte, err error)
	// Delete will hard delete the key from the Chord network
	// Note that Put/Get methods can share the same keyspace as Prefix methods, and
	// Delete will not remove the Prefix children.
	Delete(key []byte) error

	// PrefixAppend appends the child under the prefix. This is useful for tracking
	// hierarchical structure such as directories.
	// If the child already exist, an ErrKVPrefixConflict error is returned.
	// Note that Prefix methods can share the same keyspace as Put/Get, and
	// Delete will not remove the Prefix children.
	PrefixAppend(prefix []byte, child []byte) error
	// PrefixList returns the children under the prefix.
	PrefixList(prefix []byte) (children [][]byte, err error)
	// PrefixRemove removes the matching child under the prefix.
	// If the child did not exist, this is an no-op
	PrefixRemove(prefix []byte, child []byte) error

	// Acquire obtains a lease with given time-to-live, and returns a token for
	// later renewal and release.
	// On conflicting lease acquisition, ErrKVLeaseConflict error is returned.
	// Not to be confused with memory ordering acquire/release semantics.
	Acquire(lease []byte, ttl time.Duration) (token uint64, err error)
	// Renew extends the lease with given time-to-live, given that prevToken
	// is still valid. If the renewal occurs after a previous acquire
	// has expired and a different lease was acquired, ErrKVLeaseExpired error is returned.
	Renew(lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error)
	// Release relinquish the lease held previously by the given token.
	// If the lease holder has changed, ErrKVLeaseExpired error is returned.
	// Not to be confused with memory ordering acquire/release semantics.
	Release(lease []byte, token uint64) error

	// Import is used when a node is transferring its KV to a remote node.
	// Used when a new node joins or a node leaves gracefully
	Import(keys [][]byte, values []*protocol.KVTransfer) error
}

type KVProvider interface {
	KV

	// Export is used when a Local node is retriving relavent keys to transfer.
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
