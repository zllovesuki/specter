package node

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"specter/spec/chord"

	zapAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

var (
	nilNode = &atomicVNode{Node: nil}
)

type atomicVNode struct {
	Node chord.VNode
}

type atomicVNodeList struct {
	Nodes []chord.VNode
}

type LocalNode struct {
	NodeConfig

	_           [64]byte
	predecessor atomic.Value // *atomicVNode
	_           [64]byte

	_          [64]byte
	successors atomic.Value // *atomicVNodeList
	_          [64]byte
	succXOR    *zapAtomic.Uint64

	fingers []struct {
		_ [64]byte
		n atomic.Value // *atomicVNode
		_ [64]byte
	}

	started    *zapAtomic.Bool
	stopCtx    context.Context
	cancelFunc context.CancelFunc

	kv chord.KV
}

var _ chord.VNode = (*LocalNode)(nil)

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		NodeConfig: conf,
		succXOR:    zapAtomic.NewUint64(conf.Identity.GetId()),
		started:    zapAtomic.NewBool(false),
		kv:         conf.KVProvider,
		fingers: make([]struct {
			_ [64]byte
			n atomic.Value
			_ [64]byte
		}, chord.MaxFingerEntries+1),
	}
	for i := range n.fingers {
		n.fingers[i].n.Store(&atomicVNode{})
	}
	n.stopCtx, n.cancelFunc = context.WithCancel(context.Background())

	return n
}

func (n *LocalNode) Create() error {
	if !n.started.CAS(false, true) {
		return fmt.Errorf("chord node already started")
	}

	n.Logger.Info("Creating new Chord ring",
		zap.Uint64("node", n.ID()),
	)

	n.predecessor.Store(nilNode)

	s := &atomicVNodeList{
		Nodes: makeList(n, []chord.VNode{}),
	}
	n.succXOR.Store(n.xor(s.Nodes))
	n.successors.Store(s)

	n.startTasks()

	return nil
}

func (n *LocalNode) Join(peer chord.VNode) error {
	if peer.ID() == n.ID() {
		return fmt.Errorf("found duplicate node ID %d in the ring", n.ID())
	}

	proposedSucc, err := peer.FindSuccessor(n.ID())
	if err != nil {
		return fmt.Errorf("querying immediate successor: %w", err)
	}
	if proposedSucc == nil {
		return fmt.Errorf("peer has no successor")
	}
	if proposedSucc.ID() == n.ID() {
		return fmt.Errorf("found duplicate node ID %d in the ring", n.ID())
	}

	n.Logger.Info("Joining Chord ring",
		zap.Uint64("node", n.ID()),
		zap.String("via", peer.Identity().GetAddress()),
		zap.Uint64("successor", proposedSucc.ID()),
	)

	successors, err := proposedSucc.GetSuccessors()
	if err != nil {
		return fmt.Errorf("querying successor list: %w", err)
	}

	if !n.started.CAS(false, true) {
		return fmt.Errorf("chord node already started")
	}

	n.predecessor.Store(nilNode)

	s := &atomicVNodeList{
		Nodes: makeList(proposedSucc, successors),
	}
	n.succXOR.Store(n.xor(s.Nodes))
	n.successors.Store(s)

	n.startTasks()

	return nil
}

func (n *LocalNode) Stop() {
	if !n.started.CAS(true, false) {
		return
	}

	// remove ourself from the ring
	n.cancelFunc()

	n.Logger.Info("waiting for chord ring to notice our departure")
	<-time.After(n.StablizeInterval * 2)

	// then notify our successor about our predecessor
	pre, _ := n.GetPredecessor()
	succ := n.getSuccessor()

	if pre == nil {
		return
	}
	if succ == nil {
		return
	}
	if pre.ID() == n.ID() || succ.ID() == n.ID() {
		return
	}

	// relinquish control of our keys
	if err := n.transKeysOut(); err != nil {
		n.Logger.Error("transfering KV to successor", zap.Error(err))
	}

	// then notify, otherwise precedessor may not get all the keys
	if err := succ.Notify(pre); err != nil {
		n.Logger.Error("notifying successor upon leaving", zap.Error(err))
	}
}
