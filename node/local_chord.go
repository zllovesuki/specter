package node

import (
	"errors"

	"specter/spec/chord"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

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
				zap.Uint64("predecessor", new.ID()),
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
	// small optimization
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
