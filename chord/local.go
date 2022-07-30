package chord

import (
	"fmt"
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"

	"go.uber.org/atomic"
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
	_           [56]byte
	predecessor atomic.Value // *atomicVNode
	_           [56]byte
	successors  atomic.Value // *atomicVNodeList
	_           [56]byte
	succXOR     *atomic.Uint64
	_           [56]byte

	fingers []struct {
		n atomic.Value // *atomicVNode
		_ [56]byte
	}

	NodeConfig
	kv chord.KV

	lastStabilized *atomic.Time
	started        *atomic.Bool

	surrogate   chord.VNode
	surrogateMu sync.RWMutex

	stopCh chan struct{}
}

var _ chord.VNode = (*LocalNode)(nil)

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		NodeConfig: conf,
		succXOR:    atomic.NewUint64(conf.Identity.GetId()),
		started:    atomic.NewBool(false),
		kv:         conf.KVProvider,
		fingers: make([]struct {
			n atomic.Value
			_ [56]byte
		}, chord.MaxFingerEntries+1),
		lastStabilized: atomic.NewTime(time.Time{}),
		stopCh:         make(chan struct{}),
	}
	for i := range n.fingers {
		n.fingers[i].n.Store(&atomicVNode{})
	}

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

	pre, _ := n.GetPredecessor()
	succ := n.getSuccessor()

	// remove ourself from the ring
	close(n.stopCh)

	if pre == nil {
		return
	}
	if succ == nil {
		return
	}
	if pre.ID() == n.ID() || succ.ID() == n.ID() {
		return
	}

	n.Logger.Info("waiting for chord ring to notice our departure")
	<-time.After(n.StablizeInterval * 2)

	if err := n.transKeysOut(succ); err != nil {
		n.Logger.Error("transfering KV to successor", zap.Error(err))
	}

	if err := succ.Notify(pre); err != nil {
		n.Logger.Error("notifying successor upon leaving", zap.Error(err))
	}
}
