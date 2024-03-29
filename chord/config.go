package chord

import (
	"errors"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/rtt"

	"go.uber.org/zap"
)

type NodeConfig struct {
	KVProvider               chord.KVProvider
	ChordClient              rpc.ChordClient
	BaseLogger               *zap.Logger
	Identity                 *protocol.Node
	NodesRTT                 rtt.Recorder
	StabilizeInterval        time.Duration
	FixFingerInterval        time.Duration
	PredecessorCheckInterval time.Duration
}

func (c *NodeConfig) Validate() error {
	if c == nil {
		return errors.New("nil NodeConfig")
	}
	if c.BaseLogger == nil {
		return errors.New("nil BaseLogger")
	}
	if c.Identity == nil {
		return errors.New("nil Identity")
	}
	if c.Identity.GetId() >= (1 << chord.MaxFingerEntries) {
		return errors.New("invalid Identity ID")
	}
	if c.ChordClient == nil {
		return errors.New("nil ChordClient")
	}
	if c.StabilizeInterval <= 0 {
		return errors.New("invalid StabilizeInterval, must be positive")
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
	if c.NodesRTT == nil {
		return errors.New("nil NodesRTT")
	}
	return nil
}
