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
	as.Nil(err)

	rRPC := new(mocks.RPC)
	rRPC.On("Call", mock.Anything, mock.Anything).Return(&protocol.RPC_Response{}, nil)

	rpcChan := make(chan *transport.StreamDelegate)
	tp.On("RPC").Return(rpcChan)
	tp.On("DialRPC", mock.Anything, mock.Anything, mock.Anything).Return(rRPC, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go node.HandleRPC(ctx)

	c1, c2 := net.Pipe()

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
			Kind: protocol.RPC_NOTIFY,
			NotifyRequest: &protocol.NotifyRequest{
				Predecessor: &protocol.Node{
					Id: chord.Random(),
				},
			},
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
				Op:      protocol.KVOperation_LOCAL_KEYS,
				LowKey:  0,
				HighKey: 0,
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:     protocol.KVOperation_LOCAL_PUTS,
				Keys:   [][]byte{[]byte("k")},
				Values: [][]byte{[]byte("v")},
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:   protocol.KVOperation_LOCAL_GETS,
				Keys: [][]byte{[]byte("k")},
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:   protocol.KVOperation_LOCAL_DELETES,
				Keys: [][]byte{[]byte("k")},
			},
		},
	}

	for _, tc := range calls {
		_, err := caller.Call(ctx, tc)
		as.Nil(err)
	}

	node.Stop()

	_, err = caller.Call(ctx, &protocol.RPC_Request{})
	as.ErrorContains(err, ErrLeft.Error())

}
