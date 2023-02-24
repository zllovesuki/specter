package chord

import (
	"context"
	"fmt"

	"github.com/avast/retry-go/v4"
	"kon.nect.sh/specter/spec/chord"

	"go.uber.org/zap"
)

const (
	maxAttempts = 10
)

func (n *LocalNode) Create() error {
	if _, ok := n.state.Transition(chord.Inactive, chord.Joining); !ok {
		return fmt.Errorf("node is not Inactive")
	}

	n.logger.Info("Creating new Chord ring")

	successors := chord.MakeSuccListByID(n, []chord.VNode{}, chord.ExtendedSuccessorEntries)
	n.successorsMu.Lock()
	n.succListHash.Store(n.hash(successors))
	n.successors = successors
	n.successorsMu.Unlock()

	for i := 1; i <= chord.MaxFingerEntries; i++ {
		n.fingers[i].computeUpdate(func(entry *fingerEntry) {
			entry.node = n
		})
	}

	n.startTasks()

	n.state.Set(chord.Active)

	return nil
}

func (n *LocalNode) Join(peer chord.VNode) error {
	if _, ok := n.state.Transition(chord.Inactive, chord.Joining); !ok {
		return fmt.Errorf("node is not Inactive")
	}

	predecessor, successors, err := n.executeJoin(peer)
	if err != nil {
		n.state.Set(chord.Inactive)
		return err
	}

	n.successorsMu.Lock()
	n.succListHash.Store(n.hash(successors))
	n.successors = successors
	n.successorsMu.Unlock()

	n.predecessorMu.Lock()
	n.predecessor = predecessor
	n.predecessorMu.Unlock()

	n.startTasks()

	n.logger.Info("Successfully joined Chord ring", zap.Object("predecessor", predecessor.Identity()), zap.Object("successor", successors[0].Identity()))

	if err := predecessor.FinishJoin(true, false); err != nil { // advisory to let predecessor update successor list
		n.logger.Warn("error sending advisory to predecessor", zap.Error(err))
	}
	n.state.Set(chord.Active)                                     // release local join lock
	if err := successors[0].FinishJoin(false, true); err != nil { // release successor join lock
		n.logger.Warn("error releasing join lock in successor", zap.Error(err))
	}

	return nil
}

func (n *LocalNode) executeJoin(peer chord.VNode) (predecessor chord.VNode, successors []chord.VNode, err error) {
	err = retry.Do(func() error {
		var joinErr error
		n.logger.Info("Joining Chord ring",
			zap.Object("via", peer.Identity()),
		)
		predecessor, successors, joinErr = peer.RequestToJoin(n)
		return joinErr
	},
		retry.Attempts(maxAttempts),
		retry.Delay(n.StabilizeInterval),
		retry.LastErrorOnly(true),
		retry.RetryIf(chord.ErrorIsRetryable),
		retry.OnRetry(func(attempt uint, err error) {
			n.logger.Warn("Retrying on join error", zap.Uint("attempt", attempt), zap.Error(err))
		}),
	)
	return
}

func (n *LocalNode) RequestToJoin(joiner chord.VNode) (chord.VNode, []chord.VNode, error) {
	succ, err := n.FindSuccessor(joiner.ID())
	if err != nil {
		return nil, nil, err
	}
	if succ.ID() == joiner.ID() {
		return nil, nil, chord.ErrDuplicateJoinerID
	}
	if succ.ID() != n.ID() {
		return succ.RequestToJoin(joiner)
	}

	var (
		prevPredecessor chord.VNode
		joined          bool
	)

	n.logger.Info("Incoming join request", zap.Object("joiner", joiner.Identity()))

	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()

	// change status to transferring (if allowed)
	if curr, ok := n.state.Transition(chord.Active, chord.Transferring); !ok {
		n.logger.Info("Rejecting join request because current state is not Active",
			zap.String("current", curr.String()),
			zap.Object("joiner", joiner.Identity()),
		)
		return nil, nil, chord.ErrJoinInvalidState
	}

	// TODO: instrument how long it took to grab the lock and the duration it was held for
	n.predecessorMu.Lock()
	defer n.predecessorMu.Unlock()

	defer func() {
		if joined {
			n.predecessor = joiner
			return
		}
		// joiner will request to transition.
		// this is the reverting step upon error
		n.state.Set(chord.Active)
	}()

	prevPredecessor = n.predecessor

	// see issue https://github.com/zllovesuki/specter/issues/23
	if !chord.Between(prevPredecessor.ID(), joiner.ID(), n.ID(), false) {
		return nil, nil, chord.ErrJoinInvalidSuccessor
	}

	// transfer key range to new node, and set surrogate pointer to new node.
	// paper calls for forwarding but that's too hard
	// let the caller retries
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := n.transferKeysUpward(ctx, prevPredecessor, joiner); err != nil {
		return nil, nil, chord.ErrJoinTransferFailure
	}
	joined = true
	n.surrogate = joiner.Identity()

	return prevPredecessor, chord.MakeSuccListByID(n, n.getSuccessors(), chord.ExtendedSuccessorEntries), nil
}

func (n *LocalNode) FinishJoin(stabilize bool, release bool) error {
	if stabilize {
		n.logger.Info("Join completed, joiner has requested to update pointers")
		n.stabilize()
		n.fixFinger()
	}
	if release {
		n.logger.Info("Join completed, joiner has requested to release membership lock")
		if curr, ok := n.state.Transition(chord.Transferring, chord.Active); !ok {
			n.logger.Error("Unable to release membership lock", zap.String("state", curr.String()))
			return chord.ErrJoinInvalidState
		}
	}
	return nil
}

func (n *LocalNode) RequestToLeave(leaver chord.VNode) error {
	n.logger.Info("incoming leave request", zap.Object("leaver", leaver.Identity()))

	if curr, ok := n.state.Transition(chord.Active, chord.Transferring); !ok {
		n.logger.Warn("Rejecting leave request because current state is not Active", zap.String("state", curr.String()))
		return chord.ErrLeaveInvalidState
	}
	return nil
}

func (n *LocalNode) FinishLeave(stabilize bool, release bool) error {
	if stabilize {
		n.logger.Info("Leave completed, leaver has requested to update pointers")
		n.stabilize()
		n.fixFinger()
	}
	if release {
		n.logger.Info("Leave completed, leaver has requested to release membership lock")
		if curr, ok := n.state.Transition(chord.Transferring, chord.Active); !ok {
			n.logger.Error("Unable to release membership lock", zap.String("state", curr.String()))
			return chord.ErrLeaveInvalidState
		}
	}
	return nil
}

func (n *LocalNode) Leave() {
	switch n.state.Get() {
	case chord.Inactive, chord.Leaving, chord.Left:
		return
	}

	left := false
	defer func() {
		if !left {
			return
		}
		close(n.stopCh)
		n.stopWg.Wait()
	}()

	n.logger.Info("Requesting to leave chord ring")

	var (
		pre  chord.VNode
		succ chord.VNode
	)
	err := retry.Do(func() error {
		var leaveErr error
		pre, succ, leaveErr = n.executeLeave()
		return leaveErr
	},
		retry.Attempts(maxAttempts),
		retry.Delay(n.StabilizeInterval),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(attempt uint, err error) {
			n.logger.Warn("Retrying on leave error", zap.Uint("attempt", attempt), zap.Error(err))
		}),
	)
	if err != nil {
		n.logger.Error("Unable to leave ring: out of attempts", zap.Error(err))
		return
	}

	left = true

	n.logger.Info("Sending advisory to update pointers and releasing membership locks")

	// release membership locks
	if pre != nil && pre.ID() != n.ID() {
		if err := pre.FinishLeave(true, false); err != nil { // advisory to let predecessor update successor list
			n.logger.Warn("error sending advisory to predecessor", zap.Error(err))
		}
	}
	n.state.Set(chord.Left) // release local leave lock
	if succ != nil && succ.ID() != n.ID() {
		if err := succ.FinishLeave(false, true); err != nil { // if applicable, release successor leave lock
			n.logger.Warn("error releasing leave lock in successor", zap.Error(err))
		}
	}
}

func (n *LocalNode) executeLeave() (pre, succ chord.VNode, err error) {
	pre = n.getPredecessor()
	if pre == nil {
		return nil, nil, fmt.Errorf("retrying on nil predecessor")
	}
	succ = n.getSuccessor()
	if succ == nil {
		return nil, nil, chord.ErrNodeNoSuccessor
	}
	if pre.ID() == n.ID() && succ.ID() == n.ID() {
		n.logger.Debug("Skipping key transfer to successor because we are the only one left")
		return
	}

	// paper calls for asymmetric locking, and release locks when we are retrying
	if n.ID() > succ.ID() {
		if err = succ.RequestToLeave(n); err != nil {
			return
		}
		if curr, ok := n.state.Transition(chord.Active, chord.Leaving); !ok {
			n.logger.Warn("Unable to acquire local leave lock", zap.String("state", curr.String()))
			if err := succ.FinishLeave(false, true); err != nil { // release successor lock and try again
				n.logger.Warn("error releasing leave lock in successor", zap.Error(err))
			}
			return nil, nil, chord.ErrLeaveInvalidState
		}
		n.logger.Info("Leave locks acquired (succ -> self)")
	} else {
		if curr, ok := n.state.Transition(chord.Active, chord.Leaving); !ok {
			n.logger.Warn("Unable to acquire local leave lock", zap.String("state", curr.String()))
			return nil, nil, chord.ErrLeaveInvalidState
		}
		if err := succ.RequestToLeave(n); err != nil {
			n.state.Set(chord.Active) // release local lock and try again
			return nil, nil, err
		}
		n.logger.Info("Leave locks acquired (self -> succ)")
	}

	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()

	// kv requests are now blocked
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := n.transferKeysDownward(ctx, succ); err != nil {
		n.logger.Error("Transferring KV to successor", zap.Object("successor", succ.Identity()), zap.Error(err))
		// release held lock and try again
		n.state.Set(chord.Active)
		if err := succ.FinishLeave(false, true); err != nil {
			n.logger.Warn("error releasing leave lock in successor", zap.Error(err))
		}
		return nil, nil, err
	}

	n.surrogate = n.Identity()

	return
}
