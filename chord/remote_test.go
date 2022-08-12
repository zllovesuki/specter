package chord

import (
	"context"
	"fmt"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"
	mocks "kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type errorWrapper struct {
	err error
}

var _ error = (*errorWrapper)(nil)

func (e *errorWrapper) Error() string {
	return e.err.Error()
}

func TestRemoteErrorMapper(t *testing.T) {
	as := require.New(t)

	errors := []error{
		chord.ErrNodeGone,
		chord.ErrNodeNotStarted,
		chord.ErrNodeNoSuccessor,

		chord.ErrDuplicateJoinerID,
		chord.ErrJoinInvalidState,
		chord.ErrJoinTransferFailure,
		chord.ErrLeaveInvalidState,
		chord.ErrLeaveTransferFailure,

		chord.ErrKVStaleOwnership,
		chord.ErrKVPendingTransfer,

		chord.ErrKVSimpleConflict,
		chord.ErrKVPrefixConflict,
		chord.ErrKVLeaseConflict,

		chord.ErrKVLeaseExpired,
		chord.ErrKVLeaseInvalidTTL,
	}

	call := func(_ *protocol.RPC_Response, err error) (*protocol.RPC_Response, error) {
		// type squashing
		return nil, &errorWrapper{err}
	}

	for _, real := range errors {
		_, mapped := errorMapper(call(nil, real))
		as.ErrorIs(mapped, real)
	}
}

func TestRemoteRPCErrors(t *testing.T) {
	as := require.New(t)

	logger, err := zap.NewDevelopment()
	as.NoError(err)

	tp := new(mocks.Transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peer := &protocol.Node{
		Unknown: true,
		Address: "127.0.0.1:1234",
	}

	e := fmt.Errorf("sup")

	rpcCaller := new(mocks.RPC)
	rpcCaller.On("Call", mock.Anything, mock.Anything).Return(nil, e)
	rpcCaller.On("Close").Return(nil)

	tp.On("DialRPC", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetUnknown() && n.GetAddress() == peer.GetAddress()
	}), mock.Anything).Return(rpcCaller, nil)

	r, err := NewRemoteNode(ctx, tp, logger, peer)
	as.NoError(err)

	defer r.Stop()

	err = r.Ping()
	as.ErrorContains(err, e.Error())

	p := new(mocks.VNode)
	p.On("Identity").Return(&protocol.Node{
		Id: chord.Random(),
	})

	err = r.Notify(p)
	as.ErrorContains(err, e.Error())

	p.AssertExpectations(t)

	_, err = r.FindSuccessor(chord.Random())
	as.ErrorContains(err, e.Error())

	_, err = r.GetSuccessors()
	as.ErrorContains(err, e.Error())

	_, err = r.GetPredecessor()
	as.ErrorContains(err, e.Error())

	err = r.Put([]byte("key"), []byte("val"))
	as.ErrorContains(err, e.Error())

	_, err = r.Get([]byte("key"))
	as.ErrorContains(err, e.Error())

	err = r.Delete([]byte("key"))
	as.ErrorContains(err, e.Error())

	err = r.PrefixAppend([]byte("prefix"), []byte("child"))
	as.ErrorContains(err, e.Error())

	_, err = r.PrefixList([]byte("prefix"))
	as.ErrorContains(err, e.Error())

	err = r.PrefixRemove([]byte("prefix"), []byte("child"))
	as.ErrorContains(err, e.Error())

	_, err = r.Acquire([]byte("lease"), time.Second)
	as.ErrorContains(err, e.Error())

	_, err = r.Renew([]byte("lease"), time.Second, 0)
	as.ErrorContains(err, e.Error())

	err = r.Release([]byte("lease"), 0)
	as.ErrorContains(err, e.Error())

	_, _, err = r.RequestToJoin(p)
	as.ErrorContains(err, e.Error())

	err = r.FinishJoin(true, false)
	as.ErrorContains(err, e.Error())

	err = r.Import([][]byte{[]byte("k")}, []*protocol.KVTransfer{{
		SimpleValue:    []byte("v"),
		PrefixChildren: [][]byte{[]byte("c")},
	}})
	as.ErrorContains(err, e.Error())

	tp.AssertExpectations(t)
}
