package chord

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
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
		ErrNodeNil,

		ErrDuplicateJoinerID,
		ErrJoinInvalidSuccessor,
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

	call := func(_ any, err error) (any, error) {
		// type squashing
		return nil, ErrorMapper(twirp.Internal.Error(err.Error()))
	}

	for _, real := range errors {
		_, mapped := call(nil, real)
		as.ErrorIs(mapped, real)
	}
}
