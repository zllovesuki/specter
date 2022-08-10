package chord

import (
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/chord"

	"go.uber.org/zap"
)

func (n *LocalNode) Create() error {
	if !n.state.Transition(chord.Inactive, chord.Active) {
		return fmt.Errorf("node is not inactive")
	}

	n.Logger.Info("Creating new Chord ring")

	successors := makeList(n, []chord.VNode{})
	n.succListHash.Store(n.hash(successors))
	n.successors.Store(&successors)

	n.startTasks()

	return nil
}

func (n *LocalNode) Join(peer chord.VNode) error {
	if !n.state.Transition(chord.Inactive, chord.Joining) {
		return fmt.Errorf("node is not inactive")
	}

	n.Logger.Info("Joining Chord ring",
		zap.String("via", peer.Identity().GetAddress()),
	)

	predecessor, successors, err := peer.RequestToJoin(n)
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

	predecessor.FinishJoin()
	n.state.Set(chord.Active)
	successors[0].FinishJoin()

	return nil
}

func (n *LocalNode) LockPredecessor(successor chord.VNode) error {
	if !n.state.Transition(chord.Active, chord.Transferring) {
		n.Logger.Warn("Membership lock was not granted to successor", zap.String("state", n.state.String()), zap.Uint64("successor", successor.ID()))
		return chord.ErrJoinInvalidState
	}
	n.Logger.Info("Membership lock acquired by successor", zap.Uint64("successor", successor.ID()))
	return nil
}

func (n *LocalNode) FinishJoin() error {
	n.Logger.Info("Join completed, releasing membership lock")
	n.stabilize()
	n.fixFinger()
	if !n.state.Transition(chord.Transferring, chord.Active) {
		// It is normal to have this warning when the ring size is only 1 and the 2nd node is joining
		n.Logger.Warn("Unable to release membership lock", zap.String("state", n.state.String()))
	}
	return nil
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

	// change status to transferring (if allowed)
	if !n.state.Transition(chord.Active, chord.Transferring) {
		n.Logger.Info("rejecting join request because current state is not Active", zap.Uint64("joiner", joiner.ID()))
		return nil, nil, chord.ErrJoinInvalidState
	}

	var joined bool
	defer func() {
		if joined {
			return
		}
		// joiner will unlock, revert upon error
		n.state.Set(chord.Active)
	}()

	// sanity check
	succList := makeList(n, n.getSuccessors())
	if succList[0] == nil {
		return nil, nil, chord.ErrNodeNoSuccessor
	}

	n.Logger.Info("incoming join request", zap.Uint64("joiner", joiner.ID()))

	n.predecessorMu.Lock()
	defer n.predecessorMu.Unlock()
	if n.predecessor.ID() != n.ID() {
		if err := n.predecessor.LockPredecessor(n); err != nil {
			return nil, nil, err
		}
	}

	n.surrogateMu.Lock()
	defer n.surrogateMu.Unlock()
	// transfer key range to new node, and set surrogate pointer to new node.
	// paper calls for forwarding but that's too hard
	// let the caller retries
	if err := n.transferKeysUpward(n.predecessor, joiner); err != nil {
		return nil, nil, chord.ErrJoinTransferFailure
	}
	prevPredecessor := n.predecessor
	n.predecessor = joiner
	n.surrogate = joiner.Identity()
	joined = true

	return prevPredecessor, succList, nil
}

func (n *LocalNode) Leave() {
RECHECK:
	state := n.state.Get()
	if state == chord.Left || state == chord.Inactive {
		return
	}
	if !n.state.Transition(chord.Active, chord.Leaving) {
		n.Logger.Warn("Node not in active state, retry leaving later", zap.String("state", state.String()))
		<-time.After(n.StablizeInterval)
		goto RECHECK
	}

	n.Logger.Info("Stopping Chord processing")

	// ensure no pending KV operations to our nodes can proceed while we are leaving
	n.surrogateMu.Lock()
	defer func() {
		n.surrogate = n.Identity()
		n.surrogateMu.Unlock()

		n.state.Set(chord.Left)

		// it is possible that our successor list is not up to date,
		// need to keep it running until the very end
		close(n.stopCh)
		// ensure other nodes can update their finger table/successor list
		<-time.After(n.StablizeInterval * 2)
	}()

	retries := 3
RETRY:
	succ := n.getSuccessor()
	if succ == nil || succ.ID() == n.ID() {
		n.Logger.Debug("Skipping key transfer to successor because successor is either nil or ourself")
		return
	}
	if err := n.transferKeysDownward(succ); err != nil {
		if chord.ErrorIsRetryable(err) && retries > 0 {
			n.Logger.Info("Immediate successor did not accept Import request, retrying")
			retries--
			<-time.After(n.StablizeInterval * 2)
			goto RETRY
		}
		n.Logger.Error("Transfering KV to successor", zap.Error(err), zap.Uint64("successor", succ.ID()))
	}

	pre := n.getPredecessor()
	if pre == nil || pre.ID() == n.ID() {
		return
	}

	if err := succ.Notify(pre); err != nil {
		n.Logger.Error("Notifying successor upon leaving", zap.Error(err))
	}
}
