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
	if !n.state.Transition(chord.Inactive, chord.Joining) {
		return fmt.Errorf("node is not inactive")
	}

	n.Logger.Info("Creating new Chord ring")

	successors := chord.MakeSuccList(n, []chord.VNode{}, chord.ExtendedSuccessorEntries)
	n.succListHash.Store(n.hash(successors))
	n.successors.Store(&successors)
	var self chord.VNode = n
	n.fingers[0].Store(&self)

	n.startTasks()

	n.state.Set(chord.Active)

	return nil
}

func (n *LocalNode) Join(peer chord.VNode) error {
	if !n.state.Transition(chord.Inactive, chord.Joining) {
		return fmt.Errorf("node is not inactive")
	}

	predecessor, successors, err := n.executeJoin(peer)
	if err != nil {
		n.state.Set(chord.Inactive)
		return err
	}

	n.succListHash.Store(n.hash(successors))
	n.successors.Store(&successors)
	n.predecessorMu.Lock()
	n.predecessor = predecessor
	n.predecessorMu.Unlock()

	n.startTasks()

	n.Logger.Info("Successfully joined Chord ring", zap.Uint64("predecessor", predecessor.ID()), zap.Uint64("successor", successors[0].ID()))

	if err := predecessor.FinishJoin(true, false); err != nil { // advisory to let predecessor update successor list
		n.Logger.Warn("error sending advisory to predecessor", zap.Error(err))
	}
	n.state.Set(chord.Active)                                     // release local join lock
	if err := successors[0].FinishJoin(false, true); err != nil { // release successor join lock
		n.Logger.Warn("error releasing join lock in successor", zap.Error(err))
	}

	return nil
}

func (n *LocalNode) executeJoin(peer chord.VNode) (predecessor chord.VNode, successors []chord.VNode, err error) {
	err = retry.Do(func() error {
		var joinErr error
		n.Logger.Info("Joining Chord ring",
			zap.String("via", peer.Identity().GetAddress()),
		)
		predecessor, successors, joinErr = peer.RequestToJoin(n)
		return joinErr
	},
		retry.Attempts(maxAttempts),
		retry.Delay(n.StablizeInterval),
		retry.LastErrorOnly(true),
		retry.RetryIf(chord.ErrorIsRetryable),
		retry.OnRetry(func(attempt uint, err error) {
			n.Logger.Warn("Retrying on join error", zap.Uint("attempt", attempt), zap.Error(err))
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

	n.Logger.Info("incoming join request", zap.Uint64("joiner", joiner.ID()))

	// change status to transferring (if allowed)
	if !n.state.Transition(chord.Active, chord.Transferring) {
		n.Logger.Info("rejecting join request because current state is not Active", zap.Uint64("joiner", joiner.ID()))
		return nil, nil, chord.ErrJoinInvalidState
	}

	var (
		prevPredecessor chord.VNode
		joined          bool
	)

	// transfer key range to new node, and set surrogate pointer to new node.
	// paper calls for forwarding but that's too hard
	// let the caller retries
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()

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

	if err := n.transferKeysUpward(ctx, prevPredecessor, joiner); err != nil {
		return nil, nil, chord.ErrJoinTransferFailure
	}
	joined = true
	n.surrogate = joiner.Identity()

	return prevPredecessor, chord.MakeSuccList(n, n.getSuccessors(), chord.ExtendedSuccessorEntries), nil
}

func (n *LocalNode) FinishJoin(stablize bool, release bool) error {
	if stablize {
		n.Logger.Info("Join completed, joiner has requested to update pointers")
		// ensure that stablize task won't trample over us
		n.successorsMu.Lock()
		n.stabilize(true)
		n.successorsMu.Unlock()
		n.fixFinger()
	}
	if release {
		n.Logger.Info("Join completed, joiner has requested to release membership lock")
		if !n.state.Transition(chord.Transferring, chord.Active) {
			n.Logger.Error("Unable to release membership lock", zap.String("state", n.state.Get().String()))
			return chord.ErrJoinInvalidState
		}
	}
	return nil
}

func (n *LocalNode) RequestToLeave(leaver chord.VNode) error {
	n.Logger.Info("incoming leave request", zap.Uint64("leaver", leaver.ID()))

	if !n.state.Transition(chord.Active, chord.Transferring) {
		n.Logger.Warn("rejecting leave request because current state is not Active")
		return chord.ErrLeaveInvalidState
	}
	return nil
}

func (n *LocalNode) FinishLeave(stablize bool, release bool) error {
	if stablize {
		n.Logger.Info("Leave completed, leaver has requested to update pointers")
		// ensure that stablize task won't trample over us
		n.successorsMu.Lock()
		n.stabilize(true)
		n.successorsMu.Unlock()
		n.fixFinger()
	}
	if release {
		n.Logger.Info("Leave completed, leaver has requested to release membership lock")
		if !n.state.Transition(chord.Transferring, chord.Active) {
			n.Logger.Error("Unable to release membership lock", zap.String("state", n.state.Get().String()))
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
	defer func() {
		close(n.stopCh)
		n.stopWg.Wait()
	}()

	n.Logger.Info("Requesting to leave chord ring")

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
		retry.Delay(n.StablizeInterval),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(attempt uint, err error) {
			n.Logger.Warn("Retrying on leave error", zap.Uint("attempt", attempt), zap.Error(err))
		}),
	)
	if err != nil {
		n.Logger.Error("Unable to leave ring: out of attempts", zap.Error(err))
		return
	}

	n.Logger.Info("Sending advisory to update pointers and releasing membership locks")

	// release membership locks
	if pre != nil && pre.ID() != n.ID() {
		if err := pre.FinishLeave(true, false); err != nil { // advisory to let predecessor update successor list
			n.Logger.Warn("error sending advisory to predecessor", zap.Error(err))
		}
	}
	n.state.Set(chord.Left) // release local leave lock
	if succ != nil && succ.ID() != n.ID() {
		if err := succ.FinishLeave(false, true); err != nil { // if applicable, release successor leave lock
			n.Logger.Warn("error releasing leave lock in successor", zap.Error(err))
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
		n.Logger.Debug("Skipping key transfer to successor because we are the only one left")
		return
	}

	// paper calls for asymmetric locking, and release locks when we are retrying
	if n.ID() > succ.ID() {
		if err = succ.RequestToLeave(n); err != nil {
			return
		}
		if !n.state.Transition(chord.Active, chord.Leaving) {
			if err := succ.FinishLeave(false, true); err != nil { // release successor lock and try again
				n.Logger.Warn("error releasing leave lock in successor", zap.Error(err))
			}
			return nil, nil, chord.ErrLeaveInvalidState
		}
		n.Logger.Info("Leave locks acquired (succ -> self)")
	} else {
		if !n.state.Transition(chord.Active, chord.Leaving) {
			return nil, nil, chord.ErrLeaveInvalidState
		}
		if err := succ.RequestToLeave(n); err != nil {
			n.state.Set(chord.Active) // release local lock and try again
			return nil, nil, err
		}
		n.Logger.Info("Leave locks acquired (self -> succ)")
	}

	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()

	// kv requests are now blocked
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := n.transferKeysDownward(ctx, succ); err != nil {
		n.Logger.Error("Transferring KV to successor", zap.Uint64("successor", succ.ID()), zap.Error(err))
		// release held lock and try again
		n.state.Set(chord.Active)
		if err := succ.FinishLeave(false, true); err != nil {
			n.Logger.Warn("error releasing leave lock in successor", zap.Error(err))
		}
		return nil, nil, err
	}

	n.surrogate = n.Identity()

	return
}
