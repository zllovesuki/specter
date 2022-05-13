package node

import (
	"context"
	"errors"
	"sync"
	"time"

	"specter/chord"
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

func (n *LocalNode) closestPreceedingNode(key uint64) chord.VNode {
	for i := chord.MaxFingerEntries - 1; i >= 0; i-- {
		finger := &n.fingers[i]
		finger.mu.RLock()
		if finger.n != nil {
			if chord.Between(n.ID(), finger.n.ID(), key, false) {
				finger.mu.RUnlock()
				return finger.n
			}
		}
		finger.mu.RUnlock()
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

func (n *LocalNode) checkPredecessor() error {
	n.preMutex.RLock()
	pre := n.predecessor
	n.preMutex.RUnlock()
	if pre == nil {
		return nil
	}
	err := pre.Ping()
	if err != nil {
		n.preMutex.Lock()
		n.logger.Debug("Discovered dead predecessor",
			zap.Uint64("node", n.ID()),
			zap.Uint64("old", n.predecessor.ID()),
		)
		n.predecessor = nil
		n.preMutex.Unlock()
	}
	return err
}

func (n *LocalNode) Put(key, value []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
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
	proposedSucc, err := peer.FindSuccessor(n.ID())
	n.logger.Info("Joining Chord ring",
		zap.Uint64("node", n.ID()),
		zap.Uint64("via", peer.ID()),
		zap.Uint64("successor", proposedSucc.ID()),
	)
	if err != nil {
		return err
	}

	n.succMutex.Lock()
	n.successor = proposedSucc
	n.succMutex.Unlock()

	err = proposedSucc.Notify(n)
	if err != nil {
		n.logger.Error("Joining existing Chord ring",
			zap.Error(err),
			zap.Uint64("local", n.ID()),
			zap.Uint64("remote", peer.ID()),
		)
		return err
	}

	if !n.started.CAS(false, true) {
		return errors.New("wtf")
	}

	n.startTasks()

	return nil
}

func (n *LocalNode) stablize() error {
	n.succMutex.Lock()
	defer n.succMutex.Unlock()

	if n.successor == nil {
		return errors.New("successor not found, possibly invalid Chord ring")
	}

	ss, err := n.successor.GetPredecessor()
	if err != nil {
		return err
	}
	if ss != nil && chord.Between(n.ID(), ss.ID(), n.successor.ID(), false) {
		n.logger.Debug("Discovered new successor via Stablize",
			zap.Uint64("node", n.ID()),
			zap.Uint64("new", ss.ID()),
			zap.Uint64("old", n.successor.ID()),
		)
		n.successor = ss
	}

	return n.successor.Notify(n)
}

func (n *LocalNode) fixFinger() error {
	mod := uint64(1 << chord.MaxFingerEntries)

	for next := 1; next <= chord.MaxFingerEntries; next++ {
		// split (x + y) % m into (x % m + y % m) % m to avoid overflow
		id := (n.ID()%mod + (1<<(next-1))%mod) % mod
		f, err := n.FindSuccessor(id)
		if err == nil {
			finger := &n.fingers[next-1]
			finger.mu.Lock()
			finger.n = f
			finger.mu.Unlock()
		}
	}
	return nil
}

func (n *LocalNode) startTasks() {
	go func() {
		ticker := time.NewTicker(n.conf.StablizeInterval)
		for {
			select {
			case <-ticker.C:
				if err := n.stablize(); err != nil {
					n.conf.Logger.Error("Stablize task", zap.Error(err))
				}
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping Stablize task", zap.Uint64("node", n.ID()))
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(n.conf.PredecessorCheckInterval)
		for {
			select {
			case <-ticker.C:
				n.checkPredecessor()
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping predecessor checking task", zap.Uint64("node", n.ID()))
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(n.conf.FixFingerInterval)
		for {
			select {
			case <-ticker.C:
				n.fixFinger()
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping FixFinger task", zap.Uint64("node", n.ID()))
				ticker.Stop()
				return
			}
		}
	}()
}

func (n *LocalNode) Stop() {
	if !n.started.CAS(true, false) {
		return
	}
	n.cancelFunc()
}
