package chord

import "fmt"

var (
	ErrNodeNotStarted    = fmt.Errorf("node is not running")
	ErrNodeGone          = fmt.Errorf("node is not part of the chord ring")
	ErrKVStaleOwnership  = fmt.Errorf("processing node no longer has ownership over requested key")
	ErrKVPendingTransfer = fmt.Errorf("kv transfer inprogress, state may be outdated")
	ErrKVPrefixConflict  = fmt.Errorf("child already exists under prefix")
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
	default:
		return false
	}
}
