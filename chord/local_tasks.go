package chord

import (
	"encoding/binary"
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/chord"

	"github.com/orisano/wyhash"
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
	hasher := wyhash.New(chord.MaxIdentitifer)
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
func (n *LocalNode) stabilize(hasLock bool) error {
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
			succList = chord.MakeSuccList(head, newSuccList, chord.ExtendedSuccessorEntries+1)
			modified = true

			if newSucc != nil && chord.Between(n.ID(), newSucc.ID(), head.ID(), false) {
				newSuccList, nsErr = newSucc.GetSuccessors()
				if nsErr == nil {
					succList = chord.MakeSuccList(newSucc, newSuccList, chord.ExtendedSuccessorEntries+1)
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
		if hasLock {
			n.updateSuccessorsList(listHash, succList)
		} else if n.successorsMu.TryLock() { // if join in progress, don't trample over
			n.updateSuccessorsList(listHash, succList)
			n.successorsMu.Unlock()
		} else {
			modified = false // if we cannot update successors list, don't notify
		}
	}

	if modified && len(succList) > 0 && n.checkNodeState(true) == nil { // don't re-notify our successor when we are leaving
		succ := succList[0]
		if err := succ.Notify(n); err != nil {
			n.Logger.Error("Error notifying successor about us", zap.Uint64("successor", succ.ID()), zap.Error(err))
		}
	}

	return nil
}

func (n *LocalNode) updateSuccessorsList(listHash uint64, succList []chord.VNode) {
	n.succListHash.Store(listHash)
	n.successors.Store(&succList)

	n.Logger.Info("Discovered new successors via Stablize",
		zap.Uint64s("successors", v2d(succList)),
	)
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
	old := *n.fingers[k].Swap(&f)
	if old == nil || old.ID() != f.ID() {
		updated = true
	}
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
		n.Logger.Info("FingerTable entries updated", zap.Ints("fixed", fixed))
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
			n.Logger.Info("Discovered dead predecessor",
				zap.Uint64("old", pre.ID()),
				zap.String("new", "nil"),
			)
		}
		n.predecessorMu.Unlock()
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
			if err := n.stabilize(false); err != nil {
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
	// run once
	n.stabilize(false)
	n.fixFinger()
	// then run periodically
	go n.periodicStablize()
	go n.periodicPredecessorCheck()
	go n.periodicFixFingers()
}
