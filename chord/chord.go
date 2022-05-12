package chord

import (
	"errors"
	"time"

	"go.uber.org/zap"
)

const (
	// Also known as M in the original paper
	MaxFingerEntries = 48
)

func between(low, target, high uint64, inclusive bool) bool {
	// account for loop around
	if high > low {
		return (low < target && target < high) || (inclusive && target == high)
	} else {
		return low < target || target < high || (inclusive && target == high)
	}
}

type NodeConfig struct {
	Logger                   *zap.Logger
	StablizeInterval         time.Duration
	FixFingerInterval        time.Duration
	PredecessorCheckInterval time.Duration
}

func (c *NodeConfig) Validate() error {
	if c == nil {
		return errors.New("wtf")
	}
	if c.Logger == nil {
		return errors.New("wtf")
	}
	if c.StablizeInterval <= 0 {
		return errors.New("wtf")
	}
	if c.FixFingerInterval <= 0 {
		return errors.New("wtf")
	}
	if c.PredecessorCheckInterval <= 0 {
		return errors.New("wtf")
	}
	return nil
}

func DefaultConfig() NodeConfig {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	return NodeConfig{
		Logger:                   logger,
		StablizeInterval:         time.Second,
		FixFingerInterval:        time.Second,
		PredecessorCheckInterval: time.Second * 2,
	}
}
