package chord

import (
	"errors"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"go.uber.org/zap"
)

type NodeConfig struct {
	Logger                   *zap.Logger
	Identity                 *protocol.Node
	KVProvider               chord.KVProvider
	StablizeInterval         time.Duration
	FixFingerInterval        time.Duration
	PredecessorCheckInterval time.Duration
	RPCClient                rpc.RPC
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
	if c.RPCClient == nil {
		return errors.New("nil RPC Client")
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
