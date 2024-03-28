package chord

import (
	"context"
	"errors"

	"github.com/twitchtv/twirp"
)

type Error struct {
	msg string
}

func (e *Error) Error() string {
	return e.msg
}

var (
	ErrJoinInvalidState     = errorDef("chord/membership: node cannot handle join request at the moment", true)
	ErrJoinTransferFailure  = errorDef("chord/membership: failed to transfer keys to joiner node", true)
	ErrJoinInvalidSuccessor = errorDef("chord/membership: join request was routed to the wrong successor node", true)
	ErrLeaveInvalidState    = errorDef("chord/membership: node cannot handle leave request at the moment", true)
	ErrLeaveTransferFailure = errorDef("chord/membership: failed to transfer keys to successor node", true)
	ErrKVStaleOwnership     = errorDef("chord/kv: processing node no longer has ownership over requested key", true)
	ErrKVPendingTransfer    = errorDef("chord/kv: kv transfer inprogress, state may be outdated", true)

	ErrNodeGone          = errorDef("chord: node is not part of the chord ring", false)
	ErrNodeNotStarted    = errorDef("chord: node is not running", false)
	ErrNodeNoSuccessor   = errorDef("chord: node has no successor, possibly invalid chord ring", false)
	ErrNodeNil           = errorDef("chord: node cannot be nil", false)
	ErrDuplicateJoinerID = errorDef("chord/membership: joining node has duplicate ID as its successor", false)
	ErrKVSimpleConflict  = errorDef("chord/kv: simple key was concurrently modified", false)
	ErrKVPrefixConflict  = errorDef("chord/kv: child already exists under prefix", false)
	ErrKVLeaseConflict   = errorDef("chord/kv: lease has not expired or was acquired by a different requester", false)
	ErrKVLeaseExpired    = errorDef("chord/kv: lease has expired with the given token", false)
	ErrKVLeaseInvalidTTL = errorDef("chord/kv: lease ttl must be greater than a second", false)
)

func ErrorIsRetryable(err error) bool {
	for _, e := range retryableErrs {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// this is needed because RPC call squash type information, so in call site with signature
// if err == ErrABC will fail (but err.Error() == ErrABC.Error() will work).
func ErrorMapper(err error) error {
	if err == nil {
		return err
	}

	var (
		srcErr    = err.Error()
		parsedErr = err
	)

	if twirpErr, ok := err.(twirp.Error); ok {
		srcErr = twirpErr.Msg()
	}

	if mapped, ok := errorStrMap[srcErr]; ok {
		parsedErr = mapped
	}

	return parsedErr
}

var retryableErrs []error = []error{
	context.DeadlineExceeded,
}

var errorStrMap map[string]error = map[string]error{}

func errorDef(str string, retryable bool) error {
	err := &Error{
		msg: str,
	}
	if retryable {
		retryableErrs = append(retryableErrs, err)
	}
	errorStrMap[str] = err
	return err
}
