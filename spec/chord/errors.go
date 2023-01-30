package chord

import (
	"context"
	"fmt"

	"github.com/twitchtv/twirp"
)

var (
	ErrNodeGone        = fmt.Errorf("chord: node is not part of the chord ring")
	ErrNodeNotStarted  = fmt.Errorf("chord: node is not running")
	ErrNodeNoSuccessor = fmt.Errorf("chord: node has no successor, possibly invalid chord ring")

	ErrDuplicateJoinerID    = fmt.Errorf("chord/membership: joining node has duplicate ID as its successor")
	ErrJoinInvalidState     = fmt.Errorf("chord/membership: node cannot handle join request at the moment")
	ErrJoinTransferFailure  = fmt.Errorf("chord/membership: failed to transfer keys to joiner node")
	ErrJoinInvalidSuccessor = fmt.Errorf("chord/membership: join request was routed to the wrong successor node")
	ErrLeaveInvalidState    = fmt.Errorf("chord/membership: node cannot handle leave request at the moment")
	ErrLeaveTransferFailure = fmt.Errorf("chord/membership: failed to transfer keys to successor node")

	ErrKVStaleOwnership  = fmt.Errorf("chord/kv: processing node no longer has ownership over requested key")
	ErrKVPendingTransfer = fmt.Errorf("chord/kv: kv transfer inprogress, state may be outdated")

	ErrKVSimpleConflict = fmt.Errorf("chord/kv: simple key was concurrently modified")
	ErrKVPrefixConflict = fmt.Errorf("chord/kv: child already exists under prefix")
	ErrKVLeaseConflict  = fmt.Errorf("chord/kv: lease has not expired or was acquired by a different requester")

	ErrKVLeaseExpired    = fmt.Errorf("chord/kv: lease has expired with the given token")
	ErrKVLeaseInvalidTTL = fmt.Errorf("chord/kv: lease ttl must be greater than a second")
)

func ErrorIsRetryable(err error) bool {
	switch err {
	case ErrKVStaleOwnership, ErrKVPendingTransfer,
		ErrJoinInvalidState, ErrJoinTransferFailure,
		ErrLeaveInvalidState, ErrLeaveTransferFailure,
		ErrJoinInvalidSuccessor, context.DeadlineExceeded:
		return true

	case ErrNodeGone, ErrNodeNotStarted, ErrDuplicateJoinerID, ErrNodeNoSuccessor,
		ErrKVSimpleConflict, ErrKVPrefixConflict, ErrKVLeaseConflict,
		ErrKVLeaseExpired, ErrKVLeaseInvalidTTL:
		fallthrough
	default:
		return false
	}
}

// this is needed because RPC call squash type information, so in call site with signature
// if err == ErrABC will fail (but err.Error() == ErrABC.Error() will work).
func ErrorMapper(err error) error {
	if err == nil {
		return err
	}
	var parsedErr error

	srcErr := err.Error()
	if twirpErr, ok := err.(twirp.Error); ok {
		srcErr = twirpErr.Msg()
	}

	switch srcErr {
	case ErrNodeNotStarted.Error():
		parsedErr = ErrNodeNotStarted
	case ErrNodeGone.Error():
		parsedErr = ErrNodeGone
	case ErrNodeNoSuccessor.Error():
		parsedErr = ErrNodeNoSuccessor

	case ErrDuplicateJoinerID.Error():
		parsedErr = ErrDuplicateJoinerID
	case ErrJoinInvalidState.Error():
		parsedErr = ErrJoinInvalidState
	case ErrJoinTransferFailure.Error():
		parsedErr = ErrJoinTransferFailure
	case ErrJoinInvalidSuccessor.Error():
		parsedErr = ErrJoinInvalidSuccessor
	case ErrLeaveInvalidState.Error():
		parsedErr = ErrLeaveInvalidState
	case ErrLeaveTransferFailure.Error():
		parsedErr = ErrLeaveTransferFailure

	case ErrKVStaleOwnership.Error():
		parsedErr = ErrKVStaleOwnership
	case ErrKVPendingTransfer.Error():
		parsedErr = ErrKVPendingTransfer

	case ErrKVPrefixConflict.Error():
		parsedErr = ErrKVPrefixConflict
	case ErrKVLeaseConflict.Error():
		parsedErr = ErrKVLeaseConflict
	case ErrKVSimpleConflict.Error():
		parsedErr = ErrKVSimpleConflict

	case ErrKVLeaseExpired.Error():
		parsedErr = ErrKVLeaseExpired
	case ErrKVLeaseInvalidTTL.Error():
		parsedErr = ErrKVLeaseInvalidTTL
	default:
		// passthrough
		parsedErr = err
	}
	return parsedErr
}
