package chord

import "fmt"

var (
	ErrNodeGone        = fmt.Errorf("node is not part of the chord ring")
	ErrNodeNotStarted  = fmt.Errorf("node is not running")
	ErrNodeNoSuccessor = fmt.Errorf("node has no successor, possibly invalid chord ring")

	ErrDuplicateJoinerID    = fmt.Errorf("joining node has duplicate ID as its successor")
	ErrJoinInvalidState     = fmt.Errorf("node cannot handle join request at the moment")
	ErrJoinTransferFailure  = fmt.Errorf("failed to transfer keys to joiner node")
	ErrLeaveInvalidState    = fmt.Errorf("node cannot handle leave request at the moment")
	ErrLeaveTransferFailure = fmt.Errorf("failed to transfer keys to successor node")

	ErrKVStaleOwnership  = fmt.Errorf("processing node no longer has ownership over requested key")
	ErrKVPendingTransfer = fmt.Errorf("kv transfer inprogress, state may be outdated")

	ErrKVSimpleConflict = fmt.Errorf("simple key was concurrently modified")
	ErrKVPrefixConflict = fmt.Errorf("child already exists under prefix")
	ErrKVLeaseConflict  = fmt.Errorf("lease has not expired or was acquired by a different requester")

	ErrKVLeaseExpired    = fmt.Errorf("lease has expired with the given token")
	ErrKVLeaseInvalidTTL = fmt.Errorf("lease ttl must be greater than a second")
)

func ErrorIsRetryable(err error) bool {
	switch err {
	case ErrKVStaleOwnership, ErrKVPendingTransfer,
		ErrJoinInvalidState, ErrJoinTransferFailure,
		ErrLeaveInvalidState, ErrLeaveTransferFailure:
		return true

	case ErrNodeGone, ErrNodeNotStarted, ErrDuplicateJoinerID, ErrNodeNoSuccessor,
		ErrKVSimpleConflict, ErrKVPrefixConflict, ErrKVLeaseConflict,
		ErrKVLeaseExpired, ErrKVLeaseInvalidTTL:
		fallthrough
	default:
		return false
	}
}
