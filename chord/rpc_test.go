package chord

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"kon.nect.sh/specter/kv/memory"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/acceptor"
	"kon.nect.sh/specter/util/router"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestRPC(t *testing.T) {
	as := require.New(t)

	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mocks.PipeTransport()

	client1 := rpc.DynamicChordClient(rpc.DisablePooling(ctx), t1)
	identity1 := &protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	}
	node1 := NewLocalNode(NodeConfig{
		Logger:                   logger.With(zap.Uint64("node", identity1.GetId())),
		Identity:                 identity1,
		KVProvider:               memory.WithHashFn(chord.Hash),
		FixFingerInterval:        time.Millisecond * 100,
		StablizeInterval:         time.Millisecond * 300,
		PredecessorCheckInterval: time.Millisecond * 500,
		ChordClient:              client1,
		NodesRTT:                 new(mocks.Measurement),
	})

	streamRouter1 := router.NewStreamRouter(logger, t1, nil)
	node1.AttachRouter(ctx, streamRouter1)
	go streamRouter1.Accept(ctx)

	as.NoError(node1.Create())
	defer node1.Leave()

	client2 := rpc.DynamicChordClient(rpc.DisablePooling(ctx), t2)
	caller, err := NewRemoteNode(ctx, logger, client2, identity1)
	as.NoError(err)

	as.NoError(caller.Ping())

	resp1, err := caller.FindSuccessor(chord.Random())
	as.NoError(err)
	as.Equal(identity1.GetId(), resp1.ID())

	resp2, err := caller.GetSuccessors()
	as.NoError(err)
	as.Len(resp2, 1)

	resp3, err := caller.GetPredecessor()
	as.NoError(err)
	as.NotNil(resp3)
	as.Equal(identity1.GetId(), resp3.ID())

	err = caller.Put(ctx, []byte("k"), []byte("v"))
	as.NoError(err)

	resp4, err := caller.Get(ctx, []byte("k"))
	as.NoError(err)
	as.Len(resp4, 1)

	err = caller.Import(ctx, [][]byte{[]byte("y")}, []*protocol.KVTransfer{
		{
			SimpleValue:    []byte("v"),
			PrefixChildren: [][]byte{[]byte("c")},
		},
	})
	as.NoError(err)

	err = caller.Delete(ctx, []byte("y"))
	as.NoError(err)

	err = caller.PrefixAppend(ctx, []byte("p"), []byte("c"))
	as.NoError(err)

	resp5, err := caller.PrefixList(ctx, []byte("p"))
	as.NoError(err)
	as.Len(resp5, 1)

	err = caller.PrefixRemove(ctx, []byte("p"), []byte("c"))
	as.NoError(err)

	resp6, err := caller.PrefixContains(ctx, []byte("p"), []byte("c"))
	as.NoError(err)
	as.False(resp6)

	prevToken, err := caller.Acquire(ctx, []byte("lease"), time.Second)
	as.NoError(err)

	newToken, err := caller.Renew(ctx, []byte("lease"), time.Minute, prevToken)
	as.NoError(err)

	as.NotEqual(newToken, prevToken)

	err = caller.Release(ctx, []byte("lease"), newToken)
	as.NoError(err)

	err = caller.Release(ctx, []byte("lease"), prevToken)
	as.Error(err)
}

func TestRemoteRPCContext(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	e := fmt.Errorf("sup")
	key := []byte("key")
	val := []byte("val")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := new(mocks.ChordClient)
	m.On("Put", mock.MatchedBy(func(ctx context.Context) bool {
		r := rpc.GetContext(ctx)
		return r.GetRequestTarget() == protocol.Context_KV_REPLICATION
	}), mock.MatchedBy(func(req *protocol.SimpleRequest) bool {
		return bytes.Equal(req.GetKey(), key) && bytes.Equal(req.GetValue(), val)
	})).Return(nil, e)

	nsTwirp := protocol.NewVNodeServiceServer(m)
	ksTwirp := protocol.NewKVServiceServer(m)

	rpcHandler := chi.NewRouter()
	rpcHandler.Mount(nsTwirp.PathPrefix(), rpc.ExtractSerializedContext(nsTwirp))
	rpcHandler.Mount(ksTwirp.PathPrefix(), rpc.ExtractSerializedContext(ksTwirp))

	tp := mocks.SelfTransport()

	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ReadTimeout: rpcTimeout,
		Handler:     rpcHandler,
	}

	acceptor := acceptor.NewH2Acceptor(nil)
	defer acceptor.Close()

	defer srv.Shutdown(ctx)

	go srv.Serve(acceptor)

	streamRouter := router.NewStreamRouter(logger, tp, nil)
	go streamRouter.Accept(ctx)

	streamRouter.HandleChord(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		acceptor.Handle(delegate)
	})

	// yes, setting up RPC is THAT complicated

	peer := &protocol.Node{
		Id:      1234,
		Address: "127.0.0.1:1234",
	}

	rpcCaller := rpc.DynamicChordClient(ctx, tp)
	r, err := NewRemoteNode(ctx, logger, rpcCaller, peer)
	as.NoError(err)

	defer r.Stop()

	err = r.Put(rpc.WithContext(ctx, &protocol.Context{
		RequestTarget: protocol.Context_KV_REPLICATION,
	}), key, val)
	as.ErrorContains(err, e.Error())

	m.AssertExpectations(t)
}
