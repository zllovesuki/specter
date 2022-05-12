package chord

import (
	"errors"
	"sync"

	"go.uber.org/zap"
)

type LocalNode struct {
	id     uint64
	logger *zap.Logger

	predecessor VNode
	preMutex    sync.RWMutex

	successor VNode
	succMutex sync.RWMutex

	fingers []VNode
	ftMutex sync.RWMutex
}

var _ VNode = &LocalNode{}

func NewLocalNode(id uint64, logger *zap.Logger) *LocalNode {
	n := &LocalNode{
		id:          id,
		logger:      logger,
		predecessor: nil,
		fingers:     make([]VNode, MaxFingerEntries),
	}
	n.successor = n
	return n
}

func (n *LocalNode) ID() uint64 {
	return n.id
}

func (n *LocalNode) Ping() error {
	return nil
}

func (n *LocalNode) Notify(predecessor VNode) error {
	if n.predecessor == nil || predecessor.IsBetween(n.predecessor, n) {
		n.predecessor = predecessor
	}
	return nil
}

func (n *LocalNode) FindSuccessor(key uint64) (VNode, error) {
	n.succMutex.RLock()
	succ := n.successor
	n.succMutex.RUnlock()
	// immediate successor
	if between(n.ID(), key, succ.ID()) {
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

func (n *LocalNode) closestPreceedingNode(key uint64) VNode {
	n.ftMutex.RLock()
	defer n.ftMutex.RUnlock()
	for _, finger := range n.fingers {
		if finger != nil {
			if between(n.ID(), finger.ID(), key) {
				return finger
			}
		}
	}
	// fallback to ourselves
	return n
}

func (n *LocalNode) GetPredecessor() (VNode, error) {
	n.preMutex.RLock()
	pre := n.predecessor
	n.preMutex.RUnlock()
	return pre, nil
}

func (n *LocalNode) CheckPredecessor() error {
	n.preMutex.RLock()
	pre := n.predecessor
	n.preMutex.RUnlock()
	if pre == nil {
		return nil
	}
	err := pre.Ping()
	if err != nil {
		n.preMutex.Lock()
		n.predecessor = nil
		n.preMutex.Unlock()
	}
	return err
}

func (n *LocalNode) IsBetween(low, high VNode) bool {
	return between(low.ID(), n.ID(), high.ID())
}

func (n *LocalNode) Join(peer VNode) error {
	proposedSucc, err := peer.FindSuccessor(n.ID())
	if err != nil {
		return err
	}

	n.preMutex.Lock()
	n.predecessor = nil
	n.preMutex.Unlock()

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
	}

	return err
}

func (n *LocalNode) Stablize() error {
	n.succMutex.Lock()
	defer n.succMutex.Unlock()

	ss, err := n.successor.GetPredecessor()
	if err != nil {
		return err
	}
	if ss != nil && ss.IsBetween(n, n.successor) {
		n.successor = ss
	}
	if n.successor.ID() != n.ID() {
		return n.successor.Notify(n)
	}

	return nil
}

func (n *LocalNode) FixFinger(f int) error {
	if f < 1 || f > MaxFingerEntries {
		return errors.New("invalid finger number")
	}
	i := f - 1
	// magically O(log n)
	id := n.ID() + (2 << i)

	fSucc, err := n.FindSuccessor(id)
	if err != nil {
		n.ftMutex.Lock()
		n.fingers[i] = fSucc
		n.ftMutex.Unlock()
	}
	return err
}
