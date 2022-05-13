package node

import (
	"errors"
	"time"

	"specter/chord"

	"go.uber.org/zap"
)

func (n *LocalNode) stablize() error {
	n.succMutex.Lock()

	if n.successor == nil {
		n.succMutex.Unlock()
		return errors.New("successor not found, possibly invalid Chord ring")
	}

	ss, err := n.successor.GetPredecessor()
	if err != nil {
		n.succMutex.Unlock()
		return err
	}
	if ss != nil && chord.Between(n.ID(), ss.ID(), n.successor.ID(), false) {
		n.logger.Debug("Discovered new successor via Stablize",
			zap.Uint64("node", n.ID()),
			zap.Uint64("new", ss.ID()),
			zap.Uint64("old", n.successor.ID()),
		)
		n.successor = ss
	}
	succ := n.successor
	n.succMutex.Unlock()

	return succ.Notify(n)
}

func (n *LocalNode) fixFinger() error {
	mod := uint64(1 << chord.MaxFingerEntries)
	fixed := make([]uint64, 0)

	for next := 1; next <= chord.MaxFingerEntries; next++ {
		// split (x + y) % m into (x % m + y % m) % m to avoid overflow
		id := (n.ID()%mod + (1<<next)%mod) % mod
		f, err := n.FindSuccessor(id)
		if err != nil {
			n.conf.Logger.Error("Finding Successor", zap.Int("next", next), zap.Error(err))
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

func (n *LocalNode) startTasks() {
	go func() {
		ticker := time.NewTicker(n.conf.StablizeInterval)
		for {
			select {
			case <-ticker.C:
				if err := n.stablize(); err != nil {
					n.conf.Logger.Error("Stablize task", zap.Error(err))
				}
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping Stablize task", zap.Uint64("node", n.ID()))
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(n.conf.PredecessorCheckInterval)
		for {
			select {
			case <-ticker.C:
				n.checkPredecessor()
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping predecessor checking task", zap.Uint64("node", n.ID()))
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(n.conf.FixFingerInterval)
		for {
			select {
			case <-ticker.C:
				n.fixFinger()
			case <-n.stopCtx.Done():
				n.logger.Debug("Stopping FixFinger task", zap.Uint64("node", n.ID()))
				ticker.Stop()
				return
			}
		}
	}()
}
