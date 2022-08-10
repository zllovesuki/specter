package chord

import "kon.nect.sh/specter/spec/protocol"

type VNode interface {
	KV

	ID() uint64
	Identity() *protocol.Node

	Ping() error
	Notify(predecessor VNode) error

	FindSuccessor(key uint64) (VNode, error)
	GetSuccessors() ([]VNode, error)
	GetPredecessor() (VNode, error)

	VNodeMembership
}

type VNodeMembership interface {
	RequestToJoin(joiner VNode) (predecessor VNode, succList []VNode, err error)
	FinishJoin(stablize bool, release bool) error

	RequestToLeave(leaver VNode) error
	FinishLeave(stablize bool, release bool) error
}
