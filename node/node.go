package node

import (
	"errors"
	"time"

	"go.uber.org/zap"
)

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
