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

	predecessor chord.VNode
	preMutex    sync.RWMutex

	successor chord.VNode
	succMutex sync.RWMutex

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
		conf:   conf,
		logger: conf.Logger,
		fingers: make([]struct {
			mu sync.RWMutex
			n  chord.VNode
		}, chord.MaxFingerEntries),
		started: atomic.NewBool(false),
		kv:      kv.WithChordHash(),
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
	n.preMutex.Lock()
	if n.predecessor == nil || chord.Between(n.predecessor.ID(), predecessor.ID(), n.ID(), false) {
		n.logger.Debug("Discovered new predecessor via Notify",
			zap.Uint64("node", n.ID()),
			zap.Uint64("new", predecessor.ID()),
		)

		n.predecessor = predecessor
	}
	n.preMutex.Unlock()

	return nil
}

func (n *LocalNode) FindSuccessor(key uint64) (chord.VNode, error) {
	n.succMutex.RLock()
	succ := n.successor
	n.succMutex.RUnlock()
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
	for i := chord.MaxFingerEntries - 1; i >= 0; i-- {
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
	n.conf.Logger.Debug("KV Put", zap.Binary("key", key), zap.Uint64("id", id), zap.Uint64("node", succ.ID()))
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
		return errors.New("wtf")
	}

	n.logger.Info("Creating new Chord ring",
		zap.Uint64("node", n.ID()),
	)

	n.preMutex.Lock()
	n.predecessor = nil
	n.preMutex.Unlock()

	n.succMutex.Lock()
	n.successor = n
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
		return err
	}
	if proposedSucc == nil {
		return fmt.Errorf("peer has no successor")
	}
	if proposedSucc.ID() == n.ID() {
		return fmt.Errorf("found duplicate node ID %d in the ring", n.ID())
	}

	n.logger.Info("Joining Chord ring",
		zap.Uint64("node", n.ID()),
		zap.Uint64("via", peer.ID()),
		zap.Uint64("successor", proposedSucc.ID()),
	)

	n.succMutex.Lock()
	n.successor = proposedSucc
	n.succMutex.Unlock()

	err = proposedSucc.Notify(n)
	if err != nil {
		n.logger.Error("Joining existing Chord ring",
			zap.Error(err),
			zap.Uint64("node", n.ID()),
			zap.Uint64("via", peer.ID()),
		)
		return err
	}

	if !n.started.CAS(false, true) {
		return errors.New("wtf")
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
