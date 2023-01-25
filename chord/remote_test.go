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

func TestRemoteRPCErrors(t *testing.T) {
	as := require.New(t)

	logger, err := zap.NewDevelopment()
	as.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peer := &protocol.Node{
		Id:      1234,
		Address: "127.0.0.1:1234",
	}

	e := fmt.Errorf("sup")

	rpcCaller := new(mocks.RPC)
	rpcCaller.On("Call", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == peer.GetId() && n.GetAddress() == peer.GetAddress()
	}), mock.Anything).Return(nil, e)

	r, err := NewRemoteNode(ctx, logger, rpcCaller, peer)
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

	err = r.Put(ctx, []byte("key"), []byte("val"))
	as.ErrorContains(err, e.Error())

	_, err = r.Get(ctx, []byte("key"))
	as.ErrorContains(err, e.Error())

	err = r.Delete(ctx, []byte("key"))
	as.ErrorContains(err, e.Error())

	err = r.PrefixAppend(ctx, []byte("prefix"), []byte("child"))
	as.ErrorContains(err, e.Error())

	_, err = r.PrefixList(ctx, []byte("prefix"))
	as.ErrorContains(err, e.Error())

	_, err = r.PrefixContains(ctx, []byte("prefix"), []byte("child"))
	as.ErrorContains(err, e.Error())

	err = r.PrefixRemove(ctx, []byte("prefix"), []byte("child"))
	as.ErrorContains(err, e.Error())

	_, err = r.Acquire(ctx, []byte("lease"), time.Second)
	as.ErrorContains(err, e.Error())

	_, err = r.Renew(ctx, []byte("lease"), time.Second, 0)
	as.ErrorContains(err, e.Error())

	err = r.Release(ctx, []byte("lease"), 0)
	as.ErrorContains(err, e.Error())

	_, _, err = r.RequestToJoin(p)
	as.ErrorContains(err, e.Error())

	err = r.FinishJoin(true, false)
	as.ErrorContains(err, e.Error())

	err = r.Import(ctx, [][]byte{[]byte("k")}, []*protocol.KVTransfer{{
		SimpleValue:    []byte("v"),
		PrefixChildren: [][]byte{[]byte("c")},
	}})
	as.ErrorContains(err, e.Error())
}

func TestRemoteRPCContext(t *testing.T) {
	as := require.New(t)

	logger, err := zap.NewDevelopment()
	as.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peer := &protocol.Node{
		Id:      1234,
		Address: "127.0.0.1:1234",
	}

	e := fmt.Errorf("sup")

	rpcCaller := new(mocks.RPC)
	rpcCaller.On("Call", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == peer.GetId() && n.GetAddress() == peer.GetAddress()
	}), mock.MatchedBy(func(req *protocol.RPC_Request) bool {
		return req.GetRequestContext().GetRequestTarget() == protocol.Context_KV_REPLICATION
	})).Return(nil, e)

	r, err := NewRemoteNode(ctx, logger, rpcCaller, peer)
	as.NoError(err)

	defer r.Stop()

	err = r.Put(chord.WithRequestContext(ctx, &protocol.Context{
		RequestTarget: protocol.Context_KV_REPLICATION,
	}), []byte("key"), []byte("val"))
	as.ErrorContains(err, e.Error())
}
