package chord

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

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
	_            [56]byte
	predecessor  atomic.Value // *atomicVNode
	_            [56]byte
	successors   atomic.Value // *atomicVNodeList
	_            [56]byte
	succListHash *atomic.Uint64
	_            [56]byte

	fingers []struct {
		n atomic.Value // *atomicVNode
		_ [56]byte
	}

	NodeConfig
	kv chord.KV

	lastStabilized *atomic.Time
	isRunning      *atomic.Bool

	surrogate   *protocol.Node
	surrogateMu sync.RWMutex

	stopCh chan struct{}
}

var _ chord.VNode = (*LocalNode)(nil)

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		NodeConfig:   conf,
		succListHash: atomic.NewUint64(conf.Identity.GetId()),
		isRunning:    atomic.NewBool(false),
		kv:           conf.KVProvider,
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
	if !n.isRunning.CAS(false, true) {
		return fmt.Errorf("chord node already started")
	}

	n.Logger.Info("Creating new Chord ring")

	n.predecessor.Store(nilNode)

	s := &atomicVNodeList{
		Nodes: makeList(n, []chord.VNode{}),
	}
	n.succListHash.Store(n.hash(s.Nodes))
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
		zap.String("via", peer.Identity().GetAddress()),
		zap.Uint64("successor", proposedSucc.ID()),
	)

	successors, err := proposedSucc.GetSuccessors()
	if err != nil {
		return fmt.Errorf("querying successor list: %w", err)
	}

	if !n.isRunning.CAS(false, true) {
		return fmt.Errorf("chord node already started")
	}

	n.predecessor.Store(nilNode)

	s := &atomicVNodeList{
		Nodes: makeList(proposedSucc, successors),
	}
	n.succListHash.Store(n.hash(s.Nodes))
	n.successors.Store(s)

	n.startTasks()

	return nil
}

func (n *LocalNode) Stop() {
	if !n.isRunning.CAS(true, false) {
		return
	}

	n.Logger.Info("Stopping Chord processing")

	// ensure no pending KV operations to our nodes can proceed while we are leaving
	n.surrogateMu.Lock()
	defer func() {
		n.surrogate = n.Identity()
		n.surrogateMu.Unlock()

		// it is possible that our successor list is not up to date,
		// need to keep it running until the very end
		close(n.stopCh)
		// ensure other nodes can update their finger table/successor list
		<-time.After(n.StablizeInterval * 2)
	}()

	retries := 3
RETRY:
	succ := n.getSuccessor()
	if succ == nil || succ.ID() == n.ID() {
		n.Logger.Debug("Skipping key transfer to successor because successor is either nil or ourself")
		return
	}
	// TODO: figure out a way to ensure another concurrent join to our successor will get
	// our transferred keys. Currently if Join or Leave happens independently, our code is correct.
	// However, if there are concurrent Join or Leave happening with the same successor,
	// no happens-before ordering can be guaranteed, which means the new predecessor of our successor
	// may lose the ownership of our keys (as we are leaving).
	if err := n.transferKeysDownward(succ); err != nil {
		if errors.Is(err, chord.ErrNodeGone) {
			if retries > 0 {
				n.Logger.Info("Immediate successor is not responsive, retrying")
				retries--
				<-time.After(n.StablizeInterval * 2)
				goto RETRY
			}
		}
		n.Logger.Error("Transfering KV to successor", zap.Error(err), zap.Uint64("successor", succ.ID()))
	}

	pre := n.getPredecessor()
	if pre == nil || pre.ID() == n.ID() {
		return
	}

	if err := succ.Notify(pre); err != nil {
		n.Logger.Error("Notifying successor upon leaving", zap.Error(err))
	}
}
