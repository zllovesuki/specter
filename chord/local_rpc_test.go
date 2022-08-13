package chord

import (
	"context"
	"net"
	"testing"
	"time"

	"kon.nect.sh/specter/kv/memory"
	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	mocks "kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	testRenewalKey = "ll"
	testReleaseKey = "lll"
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

	kv := memory.WithHashFn(chord.HashString)
	testRenewalToken, err := kv.Acquire([]byte(testRenewalKey), time.Second)
	as.NoError(err)

	testReleaseToken, err := kv.Acquire([]byte(testReleaseKey), time.Second)
	as.NoError(err)

	node := NewLocalNode(NodeConfig{
		Logger:                   logger,
		Identity:                 identity,
		Transport:                tp,
		KVProvider:               kv,
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
				Op:  protocol.KVOperation_SIMPLE_GET,
				Key: []byte("k"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:    protocol.KVOperation_SIMPLE_PUT,
				Key:   []byte("k"),
				Value: []byte("v"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:  protocol.KVOperation_SIMPLE_DELETE,
				Key: []byte("k"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:   protocol.KVOperation_IMPORT,
				Keys: [][]byte{[]byte("k")},
				Values: []*protocol.KVTransfer{
					{
						SimpleValue:    []byte("v"),
						PrefixChildren: [][]byte{[]byte("c")},
					},
				},
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:    protocol.KVOperation_PREFIX_APPEND,
				Key:   []byte("p"),
				Value: []byte("c"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:  protocol.KVOperation_PREFIX_LIST,
				Key: []byte("p"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:    protocol.KVOperation_PREFIX_CONTAINS,
				Key:   []byte("p"),
				Value: []byte("c"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:    protocol.KVOperation_PREFIX_REMOVE,
				Key:   []byte("p"),
				Value: []byte("c"),
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:  protocol.KVOperation_LEASE_ACQUIRE,
				Key: []byte("l"),
				Lease: &protocol.KVLease{
					Ttl: durationpb.New(time.Second),
				},
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:  protocol.KVOperation_LEASE_RENEWAL,
				Key: []byte(testRenewalKey),
				Lease: &protocol.KVLease{
					Token: testRenewalToken,
					Ttl:   durationpb.New(time.Second),
				},
			},
		},

		{
			Kind: protocol.RPC_KV,
			KvRequest: &protocol.KVRequest{
				Op:  protocol.KVOperation_LEASE_RELEASE,
				Key: []byte(testReleaseKey),
				Lease: &protocol.KVLease{
					Token: testReleaseToken,
				},
			},
		},

		{
			Kind: protocol.RPC_MEMBERSHIP_CHANGE,
			MembershipRequest: &protocol.MembershipChangeRequest{
				Op: protocol.MembershipChangeOperation_JOIN_REQUEST,
				Joiner: &protocol.Node{
					Id: chord.Random(),
				},
			},
		},

		{
			Kind: protocol.RPC_MEMBERSHIP_CHANGE,
			MembershipRequest: &protocol.MembershipChangeRequest{
				Op:       protocol.MembershipChangeOperation_JOIN_FINISH,
				Stablize: true,
				Release:  true, // otherwise .Leave() will not succeed because we acquired the lock above
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

	node.Leave()

	_, err = errorMapper(caller.Call(ctx, &protocol.RPC_Request{}))
	as.ErrorIs(err, chord.ErrNodeGone)

}
