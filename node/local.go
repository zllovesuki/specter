package node

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"specter/chord"
	"specter/kv"
	"specter/spec/protocol"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type LocalNode struct {
	logger *zap.Logger
	conf   NodeConfig

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

var _ chord.VNode = &LocalNode{}

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		conf:       conf,
		logger:     conf.Logger,
		succXOR:    atomic.NewUint64(conf.Identity.GetId()),
		successors: make([]chord.VNode, chord.ExtendedSuccessorEntries+1),
		started:    atomic.NewBool(false),
		kv:         kv.WithChordHash(),
		fingers: make([]struct {
			mu sync.RWMutex
			n  chord.VNode
		}, chord.MaxFingerEntries+1),
	}
	n.stopCtx, n.cancelFunc = context.WithCancel(context.Background())

	return n
}

func (n *LocalNode) ID() uint64 {
	return n.conf.Identity.GetId()
}

func (n *LocalNode) Identity() *protocol.Node {
	return n.conf.Identity
}

func (n *LocalNode) Ping() error {
	return nil
}

func (n *LocalNode) Notify(predecessor chord.VNode) error {
	var old chord.VNode
	var new chord.VNode

	defer func() {
		if old == nil || (new != nil && old.ID() != new.ID()) {
			n.logger.Debug("Discovered new predecessor via Notify",
				zap.Uint64("node", n.ID()),
				zap.Uint64("new", new.ID()),
			)
		}
	}()

	n.preMutex.Lock()
	old = n.predecessor
	if n.predecessor == nil {
		n.predecessor = predecessor
		new = predecessor
		n.preMutex.Unlock()
		return nil
	}

	if err := n.predecessor.Ping(); err == nil {
		if chord.Between(n.predecessor.ID(), predecessor.ID(), n.ID(), false) {
			n.predecessor = predecessor
			new = predecessor
		}
	} else {
		n.predecessor = predecessor
		new = predecessor
	}
	n.preMutex.Unlock()

	return nil
}

func (n *LocalNode) getSuccessor() chord.VNode {
	n.succMutex.RLock()
	defer n.succMutex.RUnlock()
	return n.successors[0]
}

func (n *LocalNode) FindSuccessor(key uint64) (chord.VNode, error) {
	succ := n.getSuccessor()
	if succ == nil {
		return nil, errors.New("successor not found, possibly invalid Chord ring")
	}
	// immediate successor
	if chord.Between(n.ID(), key, succ.ID(), true) {
		return succ, nil
	}
	// find next in ring according to finger table
	closest := n.closestPreceedingNode(key)
	if closest.ID() == n.ID() {
		return n, nil
	}
	// contact possibly remote node
	return closest.FindSuccessor(key)
}

func (n *LocalNode) fingerRange(fn func(i int, f chord.VNode) bool) {
	stop := false
	for i := chord.MaxFingerEntries; i >= 1; i-- {
		finger := &n.fingers[i]
		finger.mu.RLock()
		if finger.n != nil {
			stop = !fn(i, finger.n)
		}
		finger.mu.RUnlock()
		if stop {
			break
		}
	}
}

func (n *LocalNode) closestPreceedingNode(key uint64) chord.VNode {
	var finger chord.VNode
	n.fingerRange(func(i int, f chord.VNode) bool {
		if chord.Between(n.ID(), f.ID(), key, false) {
			finger = f
			return false
		}
		return true
	})
	if finger != nil {
		return finger
	}
	// fallback to ourselves
	return n
}

func (n *LocalNode) GetSuccessors() ([]chord.VNode, error) {
	list := make([]chord.VNode, 0, chord.ExtendedSuccessorEntries+1)
	n.succMutex.RLock()
	defer n.succMutex.RUnlock()

	for _, s := range n.successors {
		if s == nil {
			continue
		}
		list = append(list, s)
	}

	return list, nil
}

func (n *LocalNode) GetPredecessor() (chord.VNode, error) {
	n.preMutex.RLock()
	pre := n.predecessor
	n.preMutex.RUnlock()
	return pre, nil
}

func (n *LocalNode) Put(key, value []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
	// n.conf.Logger.Debug("KV Put", zap.Binary("key", key), zap.Uint64("id", id), zap.Uint64("node", succ.ID()))
	if succ.ID() == n.ID() {
		return n.kv.Put(key, value)
	}
	return succ.Put(key, value)
}

func (n *LocalNode) Get(key []byte) ([]byte, error) {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return nil, err
	}
	if succ.ID() == n.ID() {
		return n.kv.Get(key)
	}
	return succ.Get(key)
}

func (n *LocalNode) Delete(key []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
	if succ.ID() == n.ID() {
		return n.kv.Delete(key)
	}
	return succ.Delete(key)
}

func (n *LocalNode) Create() error {
	if !n.started.CAS(false, true) {
		return fmt.Errorf("chord node already started")
	}

	n.logger.Info("Creating new Chord ring",
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

	n.logger.Info("Joining Chord ring",
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
	n.cancelFunc()
}
