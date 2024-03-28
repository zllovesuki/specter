package chord

import (
	"context"
	"fmt"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"

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

	var (
		surrogateSnapshot    chord.VNode
		predecessorSnapshot  chord.VNode
		candidatePredecessor chord.VNode
	)

	defer func() {
		if candidatePredecessor == nil {
			return
		}
		n.surrogateMu.Lock()
		if surrogateSnapshot == n.surrogate {
			if candidatePredecessor.ID() == n.ID() {
				n.surrogate = nil
			} else {
				n.surrogate = candidatePredecessor
			}
		}
		n.surrogateMu.Unlock()

		n.predecessorMu.Lock()
		if predecessorSnapshot == n.predecessor {
			n.predecessor = candidatePredecessor
		}
		n.predecessorMu.Unlock()
	}()

	n.surrogateMu.RLock()
	surrogateSnapshot = n.surrogate
	n.surrogateMu.RUnlock()

	n.predecessorMu.RLock()
	predecessorSnapshot = n.predecessor
	n.predecessorMu.RUnlock()

	// predecessor has not changed
	if predecessorSnapshot != nil && predecessorSnapshot.ID() == predecessor.ID() {
		return nil
	}

	// we have no predecessor
	if predecessorSnapshot == nil {
		candidatePredecessor = predecessor
		n.logger.Info("Discovered new predecessor via Notify",
			zap.String("previous", "nil"),
			zap.Object("predecessor", predecessor.Identity()),
		)
		return nil
	}

	// new predecessor, check connectivity
	if err := predecessorSnapshot.Ping(); err == nil {
		// old predecessor is still alive, check if new predecessor is "closer"
		if chord.Between(predecessorSnapshot.ID(), predecessor.ID(), n.ID(), false) {
			candidatePredecessor = predecessor
		}
	} else {
		// old predecessor is dead
		candidatePredecessor = predecessor
	}

	if candidatePredecessor != nil {
		n.logger.Info("Discovered new predecessor via Notify",
			zap.Object("previous", predecessorSnapshot.Identity()),
			zap.Object("predecessor", candidatePredecessor.Identity()),
		)
	}

	return nil
}

func (n *LocalNode) getSuccessor() chord.VNode {
	n.successorsMu.RLock()
	s := n.successors
	n.successorsMu.RUnlock()
	if len(s) == 0 {
		return nil
	}
	return s[0]
}

func (n *LocalNode) FindSuccessor(key uint64) (chord.VNode, error) {
	if err := n.checkNodeState(false); err != nil {
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
	closest := n.closestPrecedingNode(key)
	// contact possibly remote node
	return closest.FindSuccessor(key)
}

func (n *LocalNode) fingerRangeView(fn func(k int, f chord.VNode) bool) {
	done := false
	for k := chord.MaxFingerEntries; k >= 1; k-- {
		if done {
			return
		}
		n.fingers[k].computeView(func(node chord.VNode) {
			if node != nil {
				if !fn(k, node) {
					done = true
				}
			}
		})
	}
}

func (n *LocalNode) closestPrecedingNode(key uint64) chord.VNode {
	var finger chord.VNode
	n.fingerRangeView(func(_ int, f chord.VNode) bool {
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
	n.successorsMu.RLock()
	s := n.successors
	n.successorsMu.RUnlock()

	list := make([]chord.VNode, 0, chord.ExtendedSuccessorEntries)

	for _, s := range s {
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
	var low chord.VNode

	if newPredecessor.ID() == n.ID() {
		return nil
	}

	if prevPredecessor == nil {
		low = n
	} else {
		low = prevPredecessor
	}

	if !chord.Between(low.ID(), newPredecessor.ID(), n.ID(), false) {
		n.logger.Debug("skip transferring keys to predecessor because predecessor left", zap.Object("prev", low.Identity()), zap.Object("new", newPredecessor.Identity()))
		return
	}

	keys = n.kv.RangeKeys(low.ID(), newPredecessor.ID())
	if len(keys) == 0 {
		return nil
	}

	n.logger.Info("transferring keys to new predecessor", zap.Object("predecessor", newPredecessor.Identity()), zap.Int("num_keys", len(keys)))

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

	n.logger.Info("transferring keys to successor", zap.Object("successor", successor.Identity()), zap.Int("num_keys", len(keys)))

	values := n.kv.Export(keys)

	// TODO: split into batches
	if err := successor.Import(ctx, keys, values); err != nil {
		return fmt.Errorf("storing KV to successor: %w", err)
	}

	// TODO: remove this when we implement replication
	n.kv.RemoveKeys(keys)

	return nil
}
