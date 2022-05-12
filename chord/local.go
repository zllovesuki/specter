package chord

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type LocalNode struct {
	id     uint64
	logger *zap.Logger

	predecessor VNode
	preMutex    sync.RWMutex

	successor VNode
	succMutex sync.RWMutex

	fingers []VNode
	ftMutex sync.RWMutex

	stopCtx context.Context
}

var _ VNode = &LocalNode{}

func NewLocalNode(id uint64, logger *zap.Logger) *LocalNode {
	n := &LocalNode{
		id:      id,
		logger:  logger,
		fingers: make([]VNode, MaxFingerEntries),
		stopCtx: context.Background(),
	}

	return n
}

func (n *LocalNode) ID() uint64 {
	return n.id % (1 << MaxFingerEntries)
}

func (n *LocalNode) Ping() error {
	return nil
}

func (n *LocalNode) Notify(predecessor VNode) error {
	n.preMutex.Lock()
	if n.predecessor == nil || between(n.predecessor.ID(), predecessor.ID(), n.ID(), false) {
		n.logger.Debug("Discovered new predecessor via Notify",
			zap.Uint64("node", n.ID()),
			zap.Uint64("new", predecessor.ID()),
		)

		n.predecessor = predecessor
	}
	n.preMutex.Unlock()

	return nil
}

func (n *LocalNode) FindSuccessor(key uint64) (VNode, error) {
	n.succMutex.RLock()
	succ := n.successor
	n.succMutex.RUnlock()
	if succ == nil {
		return nil, errors.New("wtf")
	}
	// immediate successor
	if between(n.ID(), key, succ.ID(), true) {
		return succ, nil
	}
	// find next in ring according to finger table
	closest := n.closestPreceedingNode(key)
	if closest.ID() == n.ID() {
		return n, nil
	}
	// contact possibly remote node
	return closest.FindSuccessor(key)
}

func (n *LocalNode) closestPreceedingNode(key uint64) VNode {
	n.ftMutex.RLock()
	defer n.ftMutex.RUnlock()
	for i := MaxFingerEntries - 1; i >= 1; i-- {
		finger := n.fingers[i-1]
		if finger != nil {
			if between(n.ID(), finger.ID(), key, true) {
				return finger
			}
		}
	}
	// fallback to ourselves
	return n
}

func (n *LocalNode) GetPredecessor() (VNode, error) {
	n.preMutex.RLock()
	pre := n.predecessor
	n.preMutex.RUnlock()
	return pre, nil
}

func (n *LocalNode) CheckPredecessor() error {
	n.preMutex.RLock()
	pre := n.predecessor
	n.preMutex.RUnlock()
	if pre == nil {
		return nil
	}
	err := pre.Ping()
	if err != nil {
		n.preMutex.Lock()
		n.logger.Debug("Discovered dead predecessor",
			zap.Uint64("node", n.ID()),
			zap.Uint64("old", n.predecessor.ID()),
		)
		n.predecessor = nil
		n.preMutex.Unlock()
	}
	return err
}

func (n *LocalNode) Create() error {
	n.logger.Info("Creating new Chord ring",
		zap.Uint64("node", n.ID()),
	)

	n.preMutex.Lock()
	n.predecessor = nil
	n.preMutex.Unlock()

	n.succMutex.Lock()
	n.successor = n
	n.succMutex.Unlock()

	return nil
}

func (n *LocalNode) Join(peer VNode) error {
	proposedSucc, err := peer.FindSuccessor(n.ID())
	n.logger.Info("Joining Chord ring",
		zap.Uint64("node", n.ID()),
		zap.Uint64("via", peer.ID()),
		zap.Uint64("successor", proposedSucc.ID()),
	)
	if err != nil {
		return err
	}

	n.succMutex.Lock()
	n.successor = proposedSucc
	n.succMutex.Unlock()

	err = proposedSucc.Notify(n)
	if err != nil {
		n.logger.Error("Joining existing Chord ring",
			zap.Error(err),
			zap.Uint64("local", n.ID()),
			zap.Uint64("remote", peer.ID()),
		)
	}

	return err
}

func (n *LocalNode) Stablize() error {
	n.succMutex.Lock()
	defer n.succMutex.Unlock()

	if n.successor == nil {
		return errors.New("wtf")
	}

	ss, err := n.successor.GetPredecessor()
	if err != nil {
		return err
	}
	if ss != nil && between(n.ID(), ss.ID(), n.successor.ID(), false) {
		n.logger.Debug("Discovered new successor via Stablize",
			zap.Uint64("node", n.ID()),
			zap.Uint64("new", ss.ID()),
			zap.Uint64("old", n.successor.ID()),
		)
		n.successor = ss
	}
	return n.successor.Notify(n)
}

func (n *LocalNode) FixFinger() error {
	mod := uint64(1 << MaxFingerEntries)

	for next := 1; next <= MaxFingerEntries; next++ {
		// split (x + y) % m into (x % m + y % m) % m to avoid overflow
		id := (n.ID()%mod + (1<<(next-1))%mod) % mod
		f, err := n.FindSuccessor(id)
		if err == nil {
			n.ftMutex.Lock()
			n.fingers[next-1] = f
			n.ftMutex.Unlock()
		}
	}
	return nil
}

func (n *LocalNode) Trace() string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(n.ID(), 10))

	var err error
	var next VNode = n

	for {
		next, err = n.FindSuccessor(next.ID() + 1)
		if err != nil {
			break
		}
		if next == nil {
			break
		}
		if next.ID() == n.ID() {
			sb.WriteString(" -> ")
			sb.WriteString(strconv.FormatUint(n.ID(), 10))
			break
		}
		sb.WriteString(" -> ")
		sb.WriteString(strconv.FormatUint(next.ID(), 10))
	}

	sb.WriteString("\n")
	return sb.String()
}

func (n *LocalNode) StartTasks() {
	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		for {
			select {
			case <-ticker.C:
				n.Stablize()
			case <-n.stopCtx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		for {
			select {
			case <-ticker.C:
				n.CheckPredecessor()
			case <-n.stopCtx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		for {
			select {
			case <-ticker.C:
				n.FixFinger()
			case <-n.stopCtx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}
