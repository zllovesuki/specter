package chord

import (
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
)

// this is needed because RPC call squash type information, so in call site with signature
// if err == ErrABC will fail (but err.Error() == ErrABC.Error() will work).
func errorMapper(resp *protocol.RPC_Response, err error) (*protocol.RPC_Response, error) {
	if err == nil {
		return resp, err
	}
	var parsedErr error
	switch err.Error() {
	case chord.ErrNodeNotStarted.Error():
		parsedErr = chord.ErrNodeNotStarted
	case chord.ErrNodeGone.Error():
		parsedErr = chord.ErrNodeGone
	case chord.ErrNodeNoSuccessor.Error():
		parsedErr = chord.ErrNodeNoSuccessor

	case chord.ErrDuplicateJoinerID.Error():
		parsedErr = chord.ErrDuplicateJoinerID
	case chord.ErrJoinInvalidState.Error():
		parsedErr = chord.ErrJoinInvalidState
	case chord.ErrJoinTransferFailure.Error():
		parsedErr = chord.ErrJoinTransferFailure
	case chord.ErrLeaveInvalidState.Error():
		parsedErr = chord.ErrLeaveInvalidState
	case chord.ErrLeaveTransferFailure.Error():
		parsedErr = chord.ErrLeaveTransferFailure

	case chord.ErrKVStaleOwnership.Error():
		parsedErr = chord.ErrKVStaleOwnership
	case chord.ErrKVPendingTransfer.Error():
		parsedErr = chord.ErrKVPendingTransfer

	case chord.ErrKVPrefixConflict.Error():
		parsedErr = chord.ErrKVPrefixConflict
	case chord.ErrKVLeaseConflict.Error():
		parsedErr = chord.ErrKVLeaseConflict
	case chord.ErrKVSimpleConflict.Error():
		parsedErr = chord.ErrKVSimpleConflict

	case chord.ErrKVLeaseExpired.Error():
		parsedErr = chord.ErrKVLeaseExpired
	case chord.ErrKVLeaseInvalidTTL.Error():
		parsedErr = chord.ErrKVLeaseInvalidTTL
	default:
		// passthrough
		parsedErr = err
	}
	return resp, parsedErr
}
