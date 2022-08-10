package chord

import (
	"fmt"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/zap"
)

func (n *LocalNode) ID() uint64 {
	return n.NodeConfig.Identity.GetId()
}

func (n *LocalNode) Identity() *protocol.Node {
	return n.NodeConfig.Identity
}

func (n *LocalNode) Ping() error {
	state := n.state.Get()
	switch state {
	case chord.Inactive:
		return chord.ErrNodeNotStarted
	case chord.Leaving, chord.Left:
		return chord.ErrNodeGone
	default:
		return nil
	}
}

func (n *LocalNode) Notify(predecessor chord.VNode) error {
	if err := n.Ping(); err != nil {
		return err
	}
	var old chord.VNode
	var new chord.VNode

	defer func() {
		if new == nil {
			return
		}
		n.surrogateMu.Lock()
		if new.ID() == n.ID() {
			n.surrogate = nil
		} else {
			n.surrogate = new.Identity()
		}
		n.surrogateMu.Unlock()
	}()

	n.predecessorMu.Lock()
	defer n.predecessorMu.Unlock()
	old = n.predecessor

	if old == nil {
		new = predecessor
		n.predecessor = predecessor
		n.Logger.Info("Discovered new predecessor via Notify",
			zap.String("previous", "nil"),
			zap.Uint64("predecessor", predecessor.ID()),
		)
		return nil
	}

	if err := old.Ping(); err == nil {
		if chord.Between(old.ID(), predecessor.ID(), n.ID(), false) {
			new = predecessor
		}
	} else {
		new = predecessor
	}

	if new != nil {
		n.predecessor = new
		n.Logger.Info("Discovered new predecessor via Notify",
			zap.Uint64("previous", old.ID()),
			zap.Uint64("predecessor", new.ID()),
		)
	}

	return nil

}

func (n *LocalNode) getSuccessor() chord.VNode {
	s := n.successors.Load()
	if s == nil {
		return nil
	}
	return (*s)[0]
}

func (n *LocalNode) FindSuccessor(key uint64) (chord.VNode, error) {
	if err := n.Ping(); err != nil {
		return nil, err
	}
	pre := n.getPredecessor()
	if pre != nil && chord.Between(pre.ID(), key, n.ID(), true) {
		return n, nil
	}
	succ := n.getSuccessor()
	if succ == nil {
		return nil, chord.ErrNodeNoSuccessor
	}
	// immediate successor
	if chord.Between(n.ID(), key, succ.ID(), true) {
		return succ, nil
	}
	// find next in ring according to finger table
	closest := n.closestPreceedingNode(key)
	// contact possibly remote node
	return closest.FindSuccessor(key)
}

func (n *LocalNode) fingerRange(fn func(k int, f chord.VNode) bool) {
	for k := chord.MaxFingerEntries; k >= 1; k-- {
		finger := *n.fingers[k].Load()
		if finger != nil {
			if !fn(k, finger) {
				break
			}
		}
	}
}

func (n *LocalNode) closestPreceedingNode(key uint64) chord.VNode {
	var finger chord.VNode
	n.fingerRange(func(_ int, f chord.VNode) bool {
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

func (n *LocalNode) getSuccessors() []chord.VNode {
	s := n.successors.Load()
	if s == nil {
		return []chord.VNode{}
	}

	list := make([]chord.VNode, 0, chord.ExtendedSuccessorEntries+1)

	for _, s := range *s {
		if s == nil {
			continue
		}
		list = append(list, s)
	}

	return list
}

func (n *LocalNode) GetSuccessors() ([]chord.VNode, error) {
	if err := n.Ping(); err != nil {
		return nil, err
	}

	return n.getSuccessors(), nil
}

func (n *LocalNode) getPredecessor() chord.VNode {
	n.predecessorMu.RLock()
	p := n.predecessor
	n.predecessorMu.RUnlock()
	return p
}

func (n *LocalNode) GetPredecessor() (chord.VNode, error) {
	if err := n.Ping(); err != nil {
		return nil, err
	}
	return n.getPredecessor(), nil
}

// transferKeyUpward is called when a new predecessor has notified us, and we should transfer predecessors'
// key range (upward). Caller of this function should hold the surrogateMu Write Lock.
func (n *LocalNode) transferKeysUpward(prevPredecessor, newPredecessor chord.VNode) (err error) {
	var keys [][]byte
	var values []*protocol.KVTransfer
	var low uint64

	if newPredecessor.ID() == n.ID() {
		return nil
	}

	if prevPredecessor == nil {
		low = n.ID()
	} else {
		low = prevPredecessor.ID()
	}

	if !chord.Between(low, newPredecessor.ID(), n.ID(), false) {
		n.Logger.Debug("skip transferring keys to predecessor because predecessor left", zap.Uint64("prev", low), zap.Uint64("new", newPredecessor.ID()))
		return
	}

	keys = n.kv.RangeKeys(low, newPredecessor.ID())
	if len(keys) == 0 {
		return nil
	}

	n.Logger.Info("transferring keys to new predecessor", zap.Uint64("predecessor", newPredecessor.ID()), zap.Int("num_keys", len(keys)))

	values = n.kv.Export(keys)

	err = newPredecessor.Import(keys, values)
	if err != nil {
		return
	}

	// TODO: remove this when we implement replication
	n.kv.RemoveKeys(keys)
	return
}

// transferKeyDownward is called when current node is leaving the ring, and we should transfer all of our keys
// to the successor (downward). Caller of this function should hold the surrogateMu Write Lock.
func (n *LocalNode) transferKeysDownward(successor chord.VNode) error {
	keys := n.kv.RangeKeys(0, 0)

	if len(keys) == 0 {
		return nil
	}

	n.Logger.Info("transferring keys to successor", zap.Uint64("successor", successor.ID()), zap.Int("num_keys", len(keys)))

	values := n.kv.Export(keys)

	// TODO: split into batches
	if err := successor.Import(keys, values); err != nil {
		return fmt.Errorf("storing KV to successor: %w", err)
	}

	return nil
}
