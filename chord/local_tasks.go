package chord

import (
	"encoding/binary"
	"fmt"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/util"

	"github.com/zeebo/xxh3"
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

// it is not safe to use xor under any ciscumstances as long as
// we have the possbility of cyclical ring that will have ourself
// in the successor list
func (n *LocalNode) hash(nodes []chord.VNode) uint64 {
	hasher := xxh3.New()
	buf := make([]byte, 8)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		binary.BigEndian.PutUint64(buf, node.ID())
		hasher.Write(buf)
	}
	return hasher.Sum64()
}

// routine based on pseudo code from the paper "How to Make Chord Correct"
func (n *LocalNode) stabilize() error {
	succList := n.getSuccessors()
	modified := false

	for len(succList) > 0 {
		head := succList[0]
		if head == nil {
			return chord.ErrNodeNoSuccessor
		}
		newSucc, spErr := head.GetPredecessor()
		newSuccList, nsErr := head.GetSuccessors()
		if spErr == nil && nsErr == nil {
			succList = chord.MakeSuccListByID(head, newSuccList, chord.ExtendedSuccessorEntries)
			modified = true

			if newSucc != nil && chord.Between(n.ID(), newSucc.ID(), head.ID(), false) {
				newSuccList, nsErr = newSucc.GetSuccessors()
				if nsErr == nil {
					succList = chord.MakeSuccListByID(newSucc, newSuccList, chord.ExtendedSuccessorEntries)
					modified = true
				}
			}
			break
		}
		n.logger.Debug("Skipping over successor", zap.Object("head", head.Identity()), zap.Uint64s("succ", v2d(succList)))
		succList = succList[1:]
	}

	n.lastStabilized.Store(time.Now())

	listHash := n.hash(succList)
	if modified && n.succListHash.Load() != listHash {
		n.successorsMu.Lock()
		n.updateSuccessorsList(listHash, succList)
		n.successorsMu.Unlock()
	}

	if modified && len(succList) > 0 && n.checkNodeState(true) == nil { // don't re-notify our successor when we are leaving
		succ := succList[0]
		if err := succ.Notify(n); err != nil {
			n.logger.Error("Error notifying successor about us", zap.Object("successor", succ.Identity()), zap.Error(err))
		}
	}

	return nil
}

func (n *LocalNode) updateSuccessorsList(listHash uint64, succList []chord.VNode) {
	n.succListHash.Store(listHash)
	n.successors = succList

	n.logger.Info("Discovered new successors via Stabilize",
		zap.Uint64s("successors", v2d(succList)),
	)
}

func (n *LocalNode) fixK(k int) (updated bool, err error) {
	var f chord.VNode
	next := chord.ModuloSum(n.ID(), 1<<(k-1))
	f, err = n.FindSuccessor(next)
	if err != nil {
		return
	}
	if f == nil {
		err = fmt.Errorf("no successor found for k = %d", k)
		return
	}
	n.fingers[k].computeUpdate(func(entry *fingerEntry) {
		if entry.node == nil || entry.node.ID() != f.ID() {
			entry.node = f
			updated = true
		}
	})
	return
}

func (n *LocalNode) fixFinger() error {
	fixed := make([]int, 0)
	for k := 1; k <= chord.MaxFingerEntries; k++ {
		changed, err := n.fixK(k)
		if err != nil {
			continue
		}
		if changed {
			fixed = append(fixed, k)
		}
	}
	if len(fixed) > 0 {
		n.logger.Info("FingerTable entries updated", zap.Ints("fixed", fixed))
	}
	return nil
}

func (n *LocalNode) checkPredecessor() error {
	n.predecessorMu.RLock()
	pre := n.predecessor
	n.predecessorMu.RUnlock()
	if pre == nil || pre.ID() == n.ID() {
		return nil
	}

	err := pre.Ping()
	if err != nil {
		n.predecessorMu.Lock()
		if n.predecessor == pre {
			n.predecessor = nil
			n.logger.Info("Discovered dead predecessor",
				zap.Object("old", pre.Identity()),
				zap.String("new", "nil"),
			)
		}
		n.predecessorMu.Unlock()
	}
	return err
}

func (n *LocalNode) periodicStabilize() {
	defer n.stopWg.Done()

	time.Sleep(n.StabilizeInterval)
	for {
		select {
		case <-n.stopCh:
			n.logger.Debug("Stopping Stabilize task")
			return
		default:
			if err := n.stabilize(); err != nil {
				n.logger.Error("Stabilize task", zap.Error(err))
			}
			time.Sleep(util.RandomTimeRange(n.StabilizeInterval))
		}
	}
}

func (n *LocalNode) periodicPredecessorCheck() {
	defer n.stopWg.Done()

	for {
		select {
		case <-n.stopCh:
			n.logger.Debug("Stopping predecessor checking task")
			return
		default:
			n.checkPredecessor()
			time.Sleep(util.RandomTimeRange(n.PredecessorCheckInterval))
		}
	}
}

func (n *LocalNode) periodicFixFingers() {
	defer n.stopWg.Done()

	time.Sleep(n.FixFingerInterval)
	for {
		select {
		case <-n.stopCh:
			n.logger.Debug("Stopping FixFinger task")
			return
		default:
			n.fixFinger()
			time.Sleep(util.RandomTimeRange(n.FixFingerInterval))
		}
	}
}

func (n *LocalNode) startTasks() {
	// run once
	n.stabilize()
	n.fixFinger()
	n.stopWg.Add(3)
	// then run periodically
	go n.periodicStabilize()
	go n.periodicPredecessorCheck()
	go n.periodicFixFingers()
}
