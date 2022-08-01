package chord

import (
	"context"
	"net"
	"testing"
	"time"

	"kon.nect.sh/specter/kv"
	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLocalRPC(t *testing.T) {
	as := require.New(t)

	logger, err := zap.NewDevelopment()
	as.Nil(err)

	identity := &protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	}

	tp := new(mocks.Transport)

	node := NewLocalNode(NodeConfig{
		Logger:                   logger,
		Identity:                 identity,
		Transport:                tp,
		KVProvider:               kv.WithChordHash(),
		FixFingerInterval:        time.Second * 3,
		StablizeInterval:         time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
	})

	err = node.Create()
	as.NoError(err)

	rRPC := new(mocks.RPC)
	rRPC.On("Call", mock.Anything, mock.Anything).Return(&protocol.RPC_Response{}, nil)

	rpcChan := make(chan *transport.StreamDelegate)
	tp.On("RPC").Return(rpcChan)
	tp.On("DialRPC", mock.Anything, mock.Anything, mock.Anything).Return(rRPC, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go node.HandleRPC(ctx)

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	rpcChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity: &protocol.Node{
			Id: chord.Random(),
		},
	}

	caller := rpc.NewRPC(logger, c2, nil)
	go caller.Start(ctx)
	defer caller.Close()

	calls := []*protocol.RPC_Request{
		{
			Kind:            protocol.RPC_IDENTITY,
			IdentityRequest: &protocol.IdentityRequest{},
		},

		{
			Kind:        protocol.RPC_PING,
			PingRequest: &protocol.PingRequest{},
		},

		{
			Kind: protocol.RPC_FIND_SUCCESSOR,
			FindSuccessorRequest: &protocol.FindSuccessorRequest{
				Key: chord.Random(),
			},
		},

		{
			Kind:                 protocol.RPC_GET_SUCCESSORS,
			GetSuccessorsRequest: &protocol.GetSuccessorsRequest{},
		},

		{
			Kind:                  protocol.RPC_GET_PREDECESSOR,
			GetPredecessorRequest: &protocol.GetPredecessorRequest{},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:  protocol.KVOperation_GET,
				Key: []byte("k"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:    protocol.KVOperation_PUT,
				Key:   []byte("k"),
				Value: []byte("v"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:  protocol.KVOperation_DELETE,
				Key: []byte("k"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:     protocol.KVOperation_DIRECT_PUTS,
				Keys:   [][]byte{[]byte("k")},
				Values: [][]byte{[]byte("v")},
			},
		},

		// since we don't have a valid ring, if we call NOTIFY before KV operations,
		// then none of the KV operations will succeed
		{
			Kind: protocol.RPC_NOTIFY,
			NotifyRequest: &protocol.NotifyRequest{
				Predecessor: &protocol.Node{
					Id: chord.Random(),
				},
			},
		},
	}

	// realistically this should take less than 1 second, but make it 3
	// in case the CI server is busy
	callCtx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()

	for _, tc := range calls {
		_, err := errorMapper(caller.Call(callCtx, tc))
		as.NoError(err)
	}

	node.Stop()

	_, err = errorMapper(caller.Call(ctx, &protocol.RPC_Request{}))
	as.ErrorIs(err, chord.ErrNodeGone)

}
