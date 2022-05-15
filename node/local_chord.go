package node

import (
	"errors"
	"fmt"

	"specter/spec/chord"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

func (n *LocalNode) ID() uint64 {
	return n.NodeConfig.Identity.GetId()
}

func (n *LocalNode) Identity() *protocol.Node {
	return n.NodeConfig.Identity
}

func (n *LocalNode) Ping() error {
	return nil
}

func (n *LocalNode) Notify(predecessor chord.VNode) error {
	var old chord.VNode
	var new chord.VNode

	defer func() {
		if old == nil || (new != nil && old.ID() != new.ID()) {
			n.Logger.Debug("Discovered new predecessor via Notify",
				zap.Uint64("node", n.ID()),
				zap.Uint64("predecessor", new.ID()),
			)
			go func() {
				if err := n.transferKeysIn(); err != nil {
					n.Logger.Error("transfer keys from successor upon notify", zap.Error(err))
				}
			}()
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

func (n *LocalNode) transferKeysIn() error {
	// find low index (predecessor)
	// find high index (this node)
	// get keys between (low, high] from successor
	// copy keys from successor
	// remove keys from successor

	pre, _ := n.GetPredecessor()
	if pre == nil {
		return fmt.Errorf("node has no predecessor")
	}

	succ := n.getSuccessor()
	if succ == nil {
		return fmt.Errorf("node has no successor")
	}

	if pre.ID() == n.ID() || succ.ID() == n.ID() {
		return nil
	}

	keys, err := succ.LocalKeys(pre.ID(), n.ID())
	if err != nil {
		return fmt.Errorf("getting keys for transfer from successor: %w", err)
	}

	n.Logger.Debug("Transfering keys from successor", zap.Int("num_keys", len(keys)), zap.Uint64("low", pre.ID()), zap.Uint64("high", succ.ID()))

	if len(keys) == 0 {
		return nil
	}

	// TODO: split into batches
	values, err := succ.LocalGets(keys)
	if err != nil {
		return fmt.Errorf("fetching values from successor: %w", err)
	}

	if err := n.LocalPuts(keys, values); err != nil {
		return fmt.Errorf("storing KV locally: %w", err)
	}

	if err := succ.LocalDeletes(keys); err != nil {
		return fmt.Errorf("removing keys from successor: %w", err)
	}

	return nil
}

func (n *LocalNode) transKeysOut() error {
	// dump all keys from local node
	// move all keys to successor

	keys, err := n.LocalKeys(0, 0)
	if err != nil {
		return fmt.Errorf("fetching all keys locally: %w", err)
	}

	n.Logger.Debug("Transfering keys to successor", zap.Int("num_keys", len(keys)))

	if len(keys) == 0 {
		return nil
	}

	values, err := n.LocalGets(keys)
	if err != nil {
		return fmt.Errorf("fetching all values locally: %w", err)
	}

	succ := n.getSuccessor()
	if succ == nil {
		return fmt.Errorf("node has no successor")
	}

	// TODO: split into batches
	if err := succ.LocalPuts(keys, values); err != nil {
		return fmt.Errorf("storing KV to successor: %w", err)
	}

	return nil
}
