package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"specter/spec/chord"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type LocalNode struct {
	NodeConfig

	preMutex    sync.RWMutex
	predecessor chord.VNode

	succMutex  sync.RWMutex
	succXOR    *atomic.Uint64
	successors []chord.VNode

	fingers []struct {
		mu sync.RWMutex
		n  chord.VNode
	}

	started    *atomic.Bool
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
		succXOR:    atomic.NewUint64(conf.Identity.GetId()),
		successors: make([]chord.VNode, chord.ExtendedSuccessorEntries+1),
		started:    atomic.NewBool(false),
		kv:         conf.KVProvider,
		fingers: make([]struct {
			mu sync.RWMutex
			n  chord.VNode
		}, chord.MaxFingerEntries+1),
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

	n.preMutex.Lock()
	n.predecessor = nil
	n.preMutex.Unlock()

	n.succMutex.Lock()
	n.successors[0] = n
	n.succXOR.Store(n.xor(n.successors))
	n.succMutex.Unlock()

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

	n.succMutex.Lock()
	n.successors[0] = proposedSucc
	copy(n.successors[1:], successors)
	n.succXOR.Store(n.xor(n.successors))
	n.succMutex.Unlock()

	if !n.started.CAS(false, true) {
		return fmt.Errorf("chord node already started")
	}

	n.startTasks()

	return nil
}

func (n *LocalNode) Stop() {
	if !n.started.CAS(true, false) {
		return
	}

	// remove ourself from the ring
	n.cancelFunc()
	<-time.After(time.Second)

	// then notify our successor about our predecessor
	pre, _ := n.GetPredecessor()
	succ := n.getSuccessor()

	if pre == nil {
		return
	}
	if pre.ID() == n.ID() {
		return
	}
	if succ == nil {
		return
	}
	if succ.ID() == n.ID() {
		return
	}

	if err := succ.Notify(pre); err != nil {
		n.Logger.Error("notifying successor upon leaving", zap.Error(err))
	}

	// then relinquish control of our keys
	if err := n.transKeysOut(); err != nil {
		n.Logger.Error("transfering KV to successor", zap.Error(err))
	}
}
