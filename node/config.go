package node

import (
	"errors"
	"time"

	"specter/overlay"
	"specter/spec/chord"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

type NodeConfig struct {
	Logger                   *zap.Logger
	Identity                 *protocol.Node
	Transport                *overlay.Transport
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
	// if c.Transport == nil {
	// 	return errors.New("nil Transport")
	// }
	if c.StablizeInterval <= 0 {
		return errors.New("invalid StablizeInterval, must be positive")
	}
	if c.FixFingerInterval <= 0 {
		return errors.New("invalid FixFingerInterval, must be positive")
	}
	if c.PredecessorCheckInterval <= 0 {
		return errors.New("invalid PredecessorCheckInterval, must be positive")
	}
	return nil
}
