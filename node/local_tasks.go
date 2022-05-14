package node

import (
	"fmt"
	"time"

	"specter/chord"

	"go.uber.org/zap"
)

func (n *LocalNode) copySuccessors() []chord.VNode {
	n.succMutex.RLock()
	succList := append([]chord.VNode(nil), n.successors...)
	n.succMutex.RUnlock()
	return succList
}

// func V2D(n []chord.VNode) []uint64 {
// 	x := make([]uint64, 0)
// 	for _, xx := range n {
// 		if xx == nil {
// 			continue
// 		}
// 		x = append(x, xx.ID())
// 	}
// 	return x
// }

func makeList(immediate chord.VNode, successors []chord.VNode) []chord.VNode {
	list := make([]chord.VNode, chord.ExtendedSuccessorEntries+1)
	list[0] = immediate
	copy(list[1:], successors)
	return list
}

func (n *LocalNode) stablize() error {
	succList := n.copySuccessors()
	modified := false

	defer func() {
		if modified {
			n.succMutex.Lock()
			copy(n.successors, succList)
			// n.logger.Debug("Current view", zap.Uint64s("successors", V2D(n.successors)))
			n.succMutex.Unlock()
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
			// n.logger.Debug("replace", zap.String("where", "head"), zap.Int("len", len(nextSuccList)))
			succList = makeList(head, nextSuccList)
			modified = true

			if newSucc != nil && chord.Between(n.ID(), newSucc.ID(), n.getSuccessor().ID(), false) {
				nextSuccList, nsErr = newSucc.GetSuccessors()
				if nsErr == nil {
					// n.logger.Debug("replace", zap.String("where", "newSucc"), zap.Int("len", len(nextSuccList)))
					succList = makeList(newSucc, nextSuccList)
					modified = true

					n.logger.Debug("Discovered new successors via Stablize",
						zap.Uint64("node", n.ID()),
						zap.Uint64("new", newSucc.ID()),
					)
					return nil
				}
			}
			break
		}
		n.logger.Debug("Skipping over successor", zap.Uint64("peer", head.ID()))
		succList = succList[1:]
	}

	return nil
}

func (n *LocalNode) fixFinger() error {
	mod := uint64(1 << chord.MaxFingerEntries)
	fixed := make([]uint64, 0)

	for next := 1; next <= chord.MaxFingerEntries; next++ {
		// split (x + y) % m into (x % m + y % m) % m to avoid overflow
		id := (n.ID()%mod + (1<<next)%mod) % mod
		f, err := n.FindSuccessor(id)
		if err != nil {
			continue
		}
		if err == nil {
			finger := &n.fingers[next-1]
			finger.mu.Lock()
			if finger.n != nil && finger.n.ID() != f.ID() {
				fixed = append(fixed, uint64(next))
			}
			finger.n = f
			finger.mu.Unlock()
		}
	}
	if len(fixed) > 0 {
		n.conf.Logger.Debug("FingerTable entries updated", zap.Uint64s("fixed", fixed))
	}
	return nil
}

func (n *LocalNode) checkPredecessor() error {
	n.preMutex.Lock()
	defer n.preMutex.Unlock()

	pre := n.predecessor
	if pre == nil {
		return nil
	}
	err := pre.Ping()
	if err != nil {
		n.logger.Debug("Discovered dead predecessor",
			zap.Uint64("node", n.ID()),
			zap.Uint64("old", n.predecessor.ID()),
		)
		n.predecessor = nil
	}
	return err
}

func (n *LocalNode) startTasks() {
	go func() {
		timer := time.NewTimer(n.conf.StablizeInterval)
		for {
			select {
			case <-timer.C:
				if err := n.stablize(); err != nil {
					n.conf.Logger.Error("Stablize task", zap.Error(err))
				}
				timer.Reset(n.conf.StablizeInterval)
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping Stablize task", zap.Uint64("node", n.ID()))
				timer.Stop()
				return
			}
		}
	}()

	go func() {
		timer := time.NewTimer(n.conf.PredecessorCheckInterval)
		for {
			select {
			case <-timer.C:
				n.checkPredecessor()
				timer.Reset(n.conf.PredecessorCheckInterval)
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping predecessor checking task", zap.Uint64("node", n.ID()))
				timer.Stop()
				return
			}
		}
	}()

	go func() {
		timer := time.NewTimer(n.conf.FixFingerInterval)
		for {
			select {
			case <-timer.C:
				n.fixFinger()
				timer.Reset(n.conf.FixFingerInterval)
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping FixFinger task", zap.Uint64("node", n.ID()))
				timer.Stop()
				return
			}
		}
	}()
}
