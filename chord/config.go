package chord

import (
	"errors"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"

	"go.uber.org/zap"
)

type KVProvider interface {
	chord.KV

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

type NodeConfig struct {
	Logger                   *zap.Logger
	Identity                 *protocol.Node
	Transport                transport.Transport
	KVProvider               KVProvider
	StablizeInterval         time.Duration
	FixFingerInterval        time.Duration
	PredecessorCheckInterval time.Duration
}

func (c *NodeConfig) Validate() error {
	if c == nil {
		return errors.New("nil NodeConfig")
	}
	if c.Logger == nil {
		return errors.New("nil Logger")
	}
	if c.Identity == nil {
		return errors.New("nil Identity")
	}
	if c.Identity.GetId() >= (1 << chord.MaxFingerEntries) {
		return errors.New("invalid Identity ID")
	}
	if c.Transport == nil {
		return errors.New("nil Transport")
	}
	if c.StablizeInterval <= 0 {
		return errors.New("invalid StablizeInterval, must be positive")
	}
	if c.FixFingerInterval <= 0 {
		return errors.New("invalid FixFingerInterval, must be positive")
	}
	if c.PredecessorCheckInterval <= 0 {
		return errors.New("invalid PredecessorCheckInterval, must be positive")
	}
	if c.KVProvider == nil {
		return errors.New("nil KVProvider")
	}
	return nil
}
