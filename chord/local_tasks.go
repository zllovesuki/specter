package chord

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/zeebo/xxh3"
	"kon.nect.sh/specter/spec/chord"

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

// previous implementation uses XOR, which becomes an issues when the succList
// becomes cyclical, causing succList never gets updated
func (n *LocalNode) hash(nodes []chord.VNode) uint64 {
	hasher := xxh3.New()
	b := make([]byte, 8)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		binary.BigEndian.PutUint64(b, node.ID())
		hasher.Write(b)
	}
	return hasher.Sum64()
}

// routine based on pseudo code from the paper "How to Make Chord Correct"
func (n *LocalNode) stabilize() error {
	succList, _ := n.GetSuccessors()
	modified := false

	for len(succList) > 0 {
		head := succList[0]
		if head == nil {
			return fmt.Errorf("no more successors for candidate, node is potentially partitioned")
		}
		newSucc, spErr := head.GetPredecessor()
		newSuccList, nsErr := head.GetSuccessors()
		if spErr == nil && nsErr == nil {
			succList = makeList(head, newSuccList)
			modified = true

			if newSucc != nil && chord.Between(n.ID(), newSucc.ID(), head.ID(), false) {
				newSuccList, nsErr = newSucc.GetSuccessors()
				if nsErr == nil {
					succList = makeList(newSucc, newSuccList)
					modified = true
				}
			}
			break
		}
		n.Logger.Debug("Skipping over successor", zap.Uint64("head", head.ID()), zap.Uint64s("succ", v2d(succList)))
		succList = succList[1:]
	}

	n.lastStabilized.Store(time.Now())

	listHash := n.hash(succList)
	if modified && n.succListHash.Load() != listHash {
		n.succListHash.Store(listHash)
		n.successors.Store(&atomicVNodeList{Nodes: succList})

		n.Logger.Info("Discovered new successors via Stablize",
			zap.Uint64s("successors", v2d(succList)),
		)
	}

	if modified && len(succList) > 0 {
		succ := succList[0]
		if err := succ.Notify(n); err != nil {
			n.Logger.Error("Error notifying successor about us", zap.Uint64("successor", succ.ID()), zap.Error(err))
		}
	}

	return nil
}

func (n *LocalNode) fixK(k int) (updated bool, err error) {
	var f chord.VNode
	next := chord.Modulo(n.ID(), 1<<(k-1))
	f, err = n.FindSuccessor(next)
	if err != nil {
		return
	}
	if f == nil {
		err = fmt.Errorf("no successor found for k = %d", k)
		return
	}
	old := n.fingers[k].Swap(&atomicVNode{Node: f}).(*atomicVNode).Node
	if old == nil || old.ID() != f.ID() {
		updated = true
	}
	return
}

func (n *LocalNode) fixFinger() error {
	fixed := make([]int, 0)
	for k := chord.MaxFingerEntries; k >= 1; k-- {
		changed, err := n.fixK(k)
		if err != nil {
			continue
		}
		if changed {
			fixed = append(fixed, k)
		}
	}
	if len(fixed) > 0 {
		n.Logger.Info("FingerTable entries updated", zap.Ints("fixed", fixed))
	}
	return nil
}

func (n *LocalNode) checkPredecessor() error {
	n.predecessorMu.Lock()
	defer n.predecessorMu.Unlock()

	pre := n.predecessor
	if pre == nil || pre.ID() == n.ID() {
		return nil
	}

	err := pre.Ping()
	if err != nil {
		n.predecessor = nil
		n.Logger.Info("Discovered dead predecessor",
			zap.Uint64("old", pre.ID()),
		)
	}
	return err
}

func (n *LocalNode) periodicStablize() {
	for {
		select {
		case <-n.stopCh:
			n.Logger.Debug("Stopping Stablize task")
			return
		default:
			if err := n.stabilize(); err != nil {
				n.Logger.Error("Stablize task", zap.Error(err))
			}
		}
		<-time.After(n.StablizeInterval)
	}
}

func (n *LocalNode) periodicPredecessorCheck() {
	for {
		select {
		case <-n.stopCh:
			n.Logger.Debug("Stopping predecessor checking task")
			return
		default:
			n.checkPredecessor()
		}
		<-time.After(n.PredecessorCheckInterval)
	}
}

func (n *LocalNode) periodicFixFingers() {
	for {
		select {
		case <-n.stopCh:
			n.Logger.Debug("Stopping FixFinger task")
			return
		default:
			n.fixFinger()
		}
		<-time.After(n.FixFingerInterval)
	}
}

func (n *LocalNode) startTasks() {
	go n.periodicStablize()
	go n.periodicPredecessorCheck()
	go n.periodicFixFingers()
}
