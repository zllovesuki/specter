package node

import (
	"errors"
	"fmt"

	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"

	"go.uber.org/zap"
)

func (n *LocalNode) ID() uint64 {
	return n.NodeConfig.Identity.GetId()
}

func (n *LocalNode) Identity() *protocol.Node {
	return n.NodeConfig.Identity
}

func (n *LocalNode) Ping() error {
	select {
	case <-n.stopCh:
		return ErrLeft
	default:
		return nil
	}
}

func (n *LocalNode) Notify(predecessor chord.VNode) error {
	var new chord.VNode
	var old chord.VNode
	var oldA *atomicVNode

	l := n.Logger.With(zap.Uint64("node", n.ID()))

	if n.predecessor.CompareAndSwap(nilNode, &atomicVNode{Node: predecessor}) {
		l.Debug("Discovered new predecessor via Notify",
			zap.String("previous", "nil"),
			zap.Uint64("predecessor", predecessor.ID()),
		)
		go n.transferKeysIn(predecessor)
		return nil
	}

	oldA = n.predecessor.Load().(*atomicVNode)
	old = oldA.Node
	if err := old.Ping(); err == nil {
		if chord.Between(old.ID(), predecessor.ID(), n.ID(), false) {
			new = predecessor
		}
	} else {
		new = predecessor
	}

	if new != nil && n.predecessor.CompareAndSwap(oldA, &atomicVNode{Node: new}) {
		l.Debug("Discovered new predecessor via Notify",
			zap.Uint64("previous", old.ID()),
			zap.Uint64("predecessor", new.ID()),
		)
		go n.transferKeysIn(new)
	}

	return nil
}

func (n *LocalNode) getSuccessor() chord.VNode {
	s := n.successors.Load()
	if s == nil {
		return nil
	}
	return s.(*atomicVNodeList).Nodes[0]
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

func (n *LocalNode) fingerRange(fn func(k int, f chord.VNode) bool) {
	for k := chord.MaxFingerEntries; k >= 1; k-- {
		finger := n.fingers[k].n.Load().(*atomicVNode).Node
		if finger != nil {
			if !fn(k, finger) {
				break
			}
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
	s := n.successors.Load()
	if s == nil {
		return []chord.VNode{}, nil
	}

	succ := s.(*atomicVNodeList).Nodes
	list := make([]chord.VNode, 0, chord.ExtendedSuccessorEntries+1)

	for _, s := range succ {
		if s == nil {
			continue
		}
		list = append(list, s)
	}

	return list, nil
}

func (n *LocalNode) getPredecessor() chord.VNode {
	a := n.predecessor.Load()
	if a == nil {
		return nil
	}
	return a.(*atomicVNode).Node
}

func (n *LocalNode) GetPredecessor() (chord.VNode, error) {
	return n.getPredecessor(), nil
}

func (n *LocalNode) transferKeysIn(predecessor chord.VNode) error {
	if predecessor.ID() == n.ID() {
		return nil
	}
	keys, err := n.kv.LocalKeys(n.ID(), predecessor.ID())
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}

	n.Logger.Info("Transfering keys to predecessor", zap.Int("num_keys", len(keys)))

	vals, err := n.kv.LocalGets(keys)
	if err != nil {
		return err
	}
	if err := predecessor.LocalPuts(keys, vals); err != nil {
		return err
	}
	return n.kv.LocalDeletes(keys)
}

func (n *LocalNode) transKeysOut(successor chord.VNode) error {
	keys, err := n.LocalKeys(0, 0)
	if err != nil {
		return fmt.Errorf("fetching all keys locally: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	values, err := n.LocalGets(keys)
	if err != nil {
		return fmt.Errorf("fetching all values locally: %w", err)
	}

	n.Logger.Info("Transfering keys to successor", zap.Int("num_keys", len(keys)), zap.Uint64("successor", successor.ID()))

	// TODO: split into batches
	if err := successor.LocalPuts(keys, values); err != nil {
		return fmt.Errorf("storing KV to successor: %w", err)
	}

	return nil
}
