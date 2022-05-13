package chord

import "specter/spec/protocol"

type VNode interface {
	KV

	ID() uint64
	Identity() *protocol.Node

	Ping() error

	Notify(predecessor VNode) error

	FindSuccessor(key uint64) (VNode, error)
	GetSuccessors() ([]VNode, error)
	GetPredecessor() (VNode, error)

	Stop()
}
