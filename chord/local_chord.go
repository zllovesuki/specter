package chord

import (
	"context"
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
func (n *LocalNode) checkNodeState(leavingIsError bool) error {
	state := n.state.Get()
	switch state {
	case chord.Inactive:
		return chord.ErrNodeNotStarted
	case chord.Leaving:
		// get around during leaving routine, successor will ping us
		// before replacing their predecessor pointer to our predecessor
		if !leavingIsError {
			return nil
		}
		return chord.ErrNodeGone
	case chord.Left:
		return chord.ErrNodeGone
	default:
		return nil
	}
}

func (n *LocalNode) Ping() error {
	return n.checkNodeState(true)
}

func (n *LocalNode) Notify(predecessor chord.VNode) error {
	if err := n.checkNodeState(false); err != nil {
		return err
	}
	var old chord.VNode
	var new chord.VNode

	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()

	defer func() {
		if new == nil {
			return
		}
		if new.ID() == n.ID() {
			n.surrogate = nil
		} else {
			n.surrogate = new.Identity()
		}
	}()

	n.predecessorMu.Lock()
	defer n.predecessorMu.Unlock()

	old = n.predecessor
	if old != nil && old.ID() == predecessor.ID() {
		return nil
	}

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
		if chord.BetweenStrict(old.ID(), predecessor.ID(), n.ID()) {
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

func (n *LocalNode) findPredecessor(key uint64) (chord.VNode, chord.VNode, error) {
	var (
		err         error
		successors  []chord.VNode
		predecessor chord.VNode = n
	)
	for {
		successors, err = predecessor.GetSuccessors()
		if err != nil {
			return nil, nil, err
		}
		if chord.BetweenInclusiveHigh(predecessor.ID(), key, successors[0].ID()) {
			break
		}
		predecessor, err = predecessor.ClosestPreceedingFinger(key)
		if err != nil {
			return nil, nil, err
		}
	}
	return predecessor, successors[0], nil
}

func (n *LocalNode) FindSuccessor(key uint64) (chord.VNode, error) {
	if err := n.checkNodeState(false); err != nil {
		return nil, err
	}
	// pre := n.getPredecessor()
	// if pre != nil && chord.BetweenInclusiveHigh(pre.ID(), key, n.ID()) {
	// 	return n, nil
	// }
	// succ := n.getSuccessor()
	// if succ == nil {
	// 	return nil, chord.ErrNodeNoSuccessor
	// }
	// // immediate successor
	// if chord.BetweenInclusiveHigh(n.ID(), key, succ.ID()) {
	// 	return succ, nil
	// }
	// // find next in ring according to finger table
	// closest := n.ClosestPreceedingNode(key)
	// // contact possibly remote node
	// return closest.FindSuccessor(key)
	_, succ, err := n.findPredecessor(key)
	if err != nil {
		return nil, err
	}
	if succ == nil {
		return nil, chord.ErrNodeNoSuccessor
	}
	return succ, nil
}

func (n *LocalNode) ClosestPreceedingFinger(key uint64) (chord.VNode, error) {
	var (
		entry     *fingerEntry
		candidate chord.VNode
	)
	for i := chord.MaxFingerEntries; i >= 1; i-- {
		entry = &n.fingers[i]
		entry.mu.RLock()
		if chord.BetweenStrict(n.ID(), entry.node.ID(), key) {
			candidate = entry.node
			entry.mu.RUnlock()
			break
		}
		entry.mu.RUnlock()
	}
	if candidate != nil {
		return candidate, nil
	}
	return n, nil
}

func (n *LocalNode) UpdateFinger(k int, node chord.VNode) error {
	if k < 1 || k > chord.MaxFingerEntries {
		return fmt.Errorf("k is out of bound")
	}
	entry := &n.fingers[k]
	entry.mu.Lock()
	if !chord.BetweenInclusiveLow(n.ID(), node.ID(), entry.node.ID()) {
		entry.mu.Unlock()
		return nil
	}
	entry.node = node
	entry.mu.Unlock()

	prev := n.getPredecessor()
	if prev != nil {
		return prev.UpdateFinger(k, node)
	}

	return nil
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
	if err := n.checkNodeState(false); err != nil {
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
	if err := n.checkNodeState(false); err != nil {
		return nil, err
	}
	return n.getPredecessor(), nil
}

// transferKeyUpward is called when a new predecessor has notified us, and we should transfer predecessors'
// key range (upward). Caller of this function should hold the surrogateMu Write Lock.
func (n *LocalNode) transferKeysUpward(ctx context.Context, prevPredecessor, newPredecessor chord.VNode) (err error) {
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

	if !chord.BetweenStrict(low, newPredecessor.ID(), n.ID()) {
		n.Logger.Debug("skip transferring keys to predecessor because predecessor left", zap.Uint64("prev", low), zap.Uint64("new", newPredecessor.ID()))
		return
	}

	keys = n.kv.RangeKeys(low, newPredecessor.ID())
	if len(keys) == 0 {
		return nil
	}

	n.Logger.Info("transferring keys to new predecessor", zap.Uint64("predecessor", newPredecessor.ID()), zap.Int("num_keys", len(keys)))

	values = n.kv.Export(keys)

	err = newPredecessor.Import(ctx, keys, values)
	if err != nil {
		return
	}

	// TODO: remove this when we implement replication
	n.kv.RemoveKeys(keys)
	return
}

// transferKeyDownward is called when current node is leaving the ring, and we should transfer all of our keys
// to the successor (downward). Caller of this function should hold the surrogateMu Write Lock.
func (n *LocalNode) transferKeysDownward(ctx context.Context, successor chord.VNode) error {
	keys := n.kv.RangeKeys(0, 0)

	if len(keys) == 0 {
		return nil
	}

	n.Logger.Info("transferring keys to successor", zap.Uint64("successor", successor.ID()), zap.Int("num_keys", len(keys)))

	values := n.kv.Export(keys)

	// TODO: split into batches
	if err := successor.Import(ctx, keys, values); err != nil {
		return fmt.Errorf("storing KV to successor: %w", err)
	}

	// TODO: remove this when we implement replication
	n.kv.RemoveKeys(keys)

	return nil
}
