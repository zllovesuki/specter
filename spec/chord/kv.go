package chord

import "kon.nect.sh/specter/spec/protocol"

type KV interface {
	// Put will store the value to a node in the Chord network responsible for the given key.
	// If the key did not exist, a new entry will be added.
	// If the key already exist, the value will be overwrriten
	Put(key, value []byte) error
	// Get will fetch the value from a node in the Chord network
	// TODO: separate nil vs not found
	Get(key []byte) (value []byte, err error)
	// Delete will hard delete the key from the Chord network
	Delete(key []byte) error

	// Import is used when a node is transferring its KV to a remote node.
	// Used when a new node joins or a node leaves gracefully
	Import(keys [][]byte, values []*protocol.KVTransfer) error
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
