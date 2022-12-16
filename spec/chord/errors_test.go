package chord

import (
	"testing"

	"github.com/stretchr/testify/require"
	"kon.nect.sh/specter/spec/protocol"
)

type errorWrapper struct {
	err error
}

var _ error = (*errorWrapper)(nil)

func (e *errorWrapper) Error() string {
	return e.err.Error()
}

func TestErrorMapper(t *testing.T) {
	as := require.New(t)

	errors := []error{
		ErrNodeGone,
		ErrNodeNotStarted,
		ErrNodeNoSuccessor,

		ErrDuplicateJoinerID,
		ErrJoinInvalidState,
		ErrJoinTransferFailure,
		ErrLeaveInvalidState,
		ErrLeaveTransferFailure,

		ErrKVStaleOwnership,
		ErrKVPendingTransfer,

		ErrKVSimpleConflict,
		ErrKVPrefixConflict,
		ErrKVLeaseConflict,

		ErrKVLeaseExpired,
		ErrKVLeaseInvalidTTL,
	}

	call := func(_ *protocol.RPC_Response, err error) (*protocol.RPC_Response, error) {
		// type squashing
		return nil, ErrorMapper(&errorWrapper{err})
	}

	for _, real := range errors {
		_, mapped := call(nil, real)
		as.ErrorIs(mapped, real)
	}
}
