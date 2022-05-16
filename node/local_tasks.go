package node

import (
	"fmt"
	"time"

	"specter/spec/chord"

	"go.uber.org/zap"
)

func v2d(n []chord.VNode) []uint64 {
	x := make([]uint64, 0)
	for _, xx := range n {
		if xx == nil {
			continue
		}
		x = append(x, xx.ID())
	}
	return x
}

func makeList(immediate chord.VNode, successors []chord.VNode) []chord.VNode {
	list := make([]chord.VNode, chord.ExtendedSuccessorEntries+1)
	list[0] = immediate
	copy(list[1:], successors)
	return list
}

func (n *LocalNode) xor(nodes []chord.VNode) uint64 {
	s := n.ID()
	for _, n := range nodes {
		if n == nil {
			continue
		}
		s ^= n.ID()
	}
	return s
}

func (n *LocalNode) stablize() error {
	succList, _ := n.GetSuccessors()
	modified := true

	defer func() {
		xor := n.xor(succList)
		if modified && n.succXOR.Load() != xor {
			n.succXOR.Store(xor)
			n.successors.Store(&atomicVNodeList{Nodes: succList})

			n.Logger.Debug("Discovered new successors via Stablize",
				zap.Uint64("node", n.ID()),
				zap.Uint64s("successors", v2d(succList)),
			)
		}
		if succ := n.getSuccessor(); succ != nil {
			succ.Notify(n)
		}
	}()

	for len(succList) > 0 {
		head := succList[0]
		if head == nil {
			return fmt.Errorf("no more successors for candidate, node is potentially partitioned")
		}
		newSucc, spErr := head.GetPredecessor()
		nextSuccList, nsErr := head.GetSuccessors()
		if spErr == nil && nsErr == nil {
			succList = makeList(head, nextSuccList)
			modified = true

			if newSucc != nil && chord.Between(n.ID(), newSucc.ID(), n.getSuccessor().ID(), false) {
				nextSuccList, nsErr = newSucc.GetSuccessors()
				if nsErr == nil {
					succList = makeList(newSucc, nextSuccList)
					modified = true

					return nil
				}
			}
			break
		}
		n.Logger.Debug("Skipping over successor", zap.Uint64("peer", head.ID()))
		succList = succList[1:]
	}

	return nil
}

func (n *LocalNode) fixFinger() error {
	fixed := make([]uint64, 0)

	for k := 1; k <= chord.MaxFingerEntries; k++ {
		next := chord.Modulo(n.ID(), 1<<(k-1))
		f, err := n.FindSuccessor(next)
		if err != nil {
			continue
		}
		if err == nil {
			oldA := n.fingers[k].n.Swap(&atomicVNode{Node: f})
			old := oldA.(*atomicVNode).Node
			if old != nil && old.ID() != f.ID() {
				fixed = append(fixed, uint64(k))
			}
		}
	}
	if len(fixed) > 0 {
		n.Logger.Debug("FingerTable entries updated", zap.Uint64s("fixed", fixed))
	}
	return nil
}

func (n *LocalNode) checkPredecessor() error {
	oldA := n.predecessor.Load()
	if oldA == nil {
		return nil
	}

	pre := oldA.(*atomicVNode).Node
	if pre == nil {
		return nil
	}
	if pre.ID() == n.ID() {
		return nil
	}

	err := pre.Ping()
	if err != nil && n.predecessor.CompareAndSwap(oldA, nilNode) {
		n.Logger.Debug("Discovered dead predecessor",
			zap.Uint64("node", n.ID()),
			zap.Uint64("old", pre.ID()),
		)
	}
	return err
}

func (n *LocalNode) startTasks() {
	go func() {
		timer := time.NewTimer(n.NodeConfig.StablizeInterval)
		for {
			select {
			case <-timer.C:
				if err := n.stablize(); err != nil {
					n.Logger.Error("Stablize task", zap.Error(err))
				}
				timer.Reset(n.NodeConfig.StablizeInterval)
			case <-n.stopCtx.Done():
				n.Logger.Debug("Stopping Stablize task", zap.Uint64("node", n.ID()))
				timer.Stop()
				return
			}
		}
	}()

	go func() {
		timer := time.NewTimer(n.NodeConfig.PredecessorCheckInterval)
		for {
			select {
			case <-timer.C:
				n.checkPredecessor()
				timer.Reset(n.NodeConfig.PredecessorCheckInterval)
			case <-n.stopCtx.Done():
				n.Logger.Debug("Stopping predecessor checking task", zap.Uint64("node", n.ID()))
				timer.Stop()
				return
			}
		}
	}()

	go func() {
		timer := time.NewTimer(n.NodeConfig.FixFingerInterval)
		for {
			select {
			case <-timer.C:
				n.fixFinger()
				timer.Reset(n.NodeConfig.FixFingerInterval)
			case <-n.stopCtx.Done():
				n.Logger.Debug("Stopping FixFinger task", zap.Uint64("node", n.ID()))
				timer.Stop()
				return
			}
		}
	}()
}
