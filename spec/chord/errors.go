package chord

import "fmt"

var (
	ErrNodeGone       = fmt.Errorf("node is not part of the chord ring")
	ErrNodeNotStarted = fmt.Errorf("node is not running")

	ErrKVStaleOwnership  = fmt.Errorf("processing node no longer has ownership over requested key")
	ErrKVPendingTransfer = fmt.Errorf("kv transfer inprogress, state may be outdated")
	ErrKVPrefixConflict  = fmt.Errorf("child already exists under prefix")

	ErrKVLeaseConflict = fmt.Errorf("lease has not expired or was acquired by a different requester")
	ErrKVLeaseExpired  = fmt.Errorf("lease has expired with the given token")
)

func ErrorIsRetryable(err error) bool {
	switch err {
	case ErrNodeGone:
		fallthrough
	case ErrKVStaleOwnership:
		fallthrough
	case ErrKVPendingTransfer:
		return true

	case ErrNodeNotStarted:
		fallthrough
	case ErrKVPrefixConflict:
		fallthrough
	case ErrKVLeaseConflict:
		fallthrough
	case ErrKVLeaseExpired:
		fallthrough
	default:
		return false
	}
}
