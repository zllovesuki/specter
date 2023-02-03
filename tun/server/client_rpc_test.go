package server

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	mocks "kon.nect.sh/specter/spec/mocks"
)

type sVNode struct {
	chord.VNode
	identity *protocol.Node
}

func (n *sVNode) ID() uint64 {
	return n.identity.GetId()
}

func (n *sVNode) Identity() *protocol.Node {
	return n.identity
}

func getVNode(n *protocol.Node) chord.VNode {
	return &sVNode{
		identity: n,
	}
}

func makeNodes(num int) []*protocol.Node {
	nodes := make([]*protocol.Node, 0)
	for i := 0; i < num; i++ {
		nodes = append(nodes, &protocol.Node{
			Id: chord.Random(),
		})
	}
	return nodes
}

func makeNodeList(num int) ([]*protocol.Node, []chord.VNode) {
	nodes := make([]*protocol.Node, num)
	list := make([]chord.VNode, num)
	for i := range nodes {
		nodes[i] = &protocol.Node{
			Id: chord.Random(),
		}
	}
	for i := range nodes {
		list[i] = getVNode(nodes[i])
	}
	return nodes, list
}

func assertNodes(got, exp []*protocol.Node) bool {
	for _, g := range got {
		for _, e := range exp {
			if g.GetId() == e.GetId() {
				return true
			}
		}
	}
	return false
}

func TestRPCRegisterClientOK(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node.On("Put", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	clientT.On("SendDatagram",
		mock.MatchedBy(func(node *protocol.Node) bool {
			return node.GetId() == cli.GetId()
		}),
		mock.MatchedBy(func(b []byte) bool {
			return bytes.Equal(b, []byte(testDatagramData))
		}),
	).Return(nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)
	resp, err := cRPC.RegisterIdentity(rpc.WithNode(ctx, cli), &protocol.RegisterIdentityRequest{
		Client: cli,
	})

	as.NoError(err)
	as.NotNil(resp.GetToken())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCRegisterClientFailed(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node.On("Put", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("failed")).Once()
	clientT.On("SendDatagram", mock.Anything, mock.Anything).Return(fmt.Errorf("failed")).Once()
	clientT.On("SendDatagram", mock.Anything, mock.Anything).Return(nil).Once()

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	requests := []*protocol.RegisterIdentityRequest{
		// missing Client
		{
			Client: nil,
		},

		// has Client but does not match the connected client
		{
			Client: &protocol.Node{
				Id: chord.Random(),
			},
		},

		// has Client but not connected
		{
			Client: cli,
		},

		// has Client and connected but KV failed
		{
			Client: cli,
		},
	}

	for _, req := range requests {
		_, err := cRPC.RegisterIdentity(rpc.WithNode(ctx, cli), req)
		as.Error(err)
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCPingOK(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientT.On("Identity").Return(tn)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	resp, err := cRPC.Ping(rpc.WithNode(ctx, cli), &protocol.ClientPingRequest{})

	as.NoError(err)
	as.Equal(testRootDomain, resp.GetApex())
	as.Equal(tn.GetId(), resp.GetNode().GetId())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCPingConditional(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(clientBuf, nil).Once()
	node.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	clientT.On("Identity").Return(tn)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	tests := []struct {
		token       *protocol.ClientToken
		expectError bool
	}{
		{
			expectError: false,
		},
		{
			token: &protocol.ClientToken{
				Token: []byte("correct"),
			},
			expectError: false,
		},
		{
			token: &protocol.ClientToken{
				Token: []byte("incorrect"),
			},
			expectError: true,
		},
	}

	for _, req := range tests {
		callCtx := rpc.WithClientToken(ctx, req.token)
		resp, err := cRPC.Ping(rpc.WithNode(callCtx, cli), &protocol.ClientPingRequest{})
		if req.expectError {
			as.Error(err)
		} else {
			as.NoError(err)
			as.Equal(testRootDomain, resp.GetApex())
		}
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCGetNodesUnique(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, err := generateToken()
	as.NoError(err)
	token := &protocol.ClientToken{
		Token: b,
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)

	pair := &protocol.TunnelDestination{
		Chord:  cht,
		Tunnel: tn,
	}
	pairBuf, err := pair.MarshalVT()
	as.Nil(err)

	node.On("ID").Return(cht.GetId())
	node.On("Identity").Return(cht)
	node.On("GetSuccessors").Return([]chord.VNode{getVNode(cht)}, nil)
	node.On("Get", mock.Anything, mock.Anything).Return(pairBuf, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	callCtx := rpc.WithClientToken(ctx, token)
	resp, err := cRPC.GetNodes(rpc.WithNode(callCtx, cli), &protocol.GetNodesRequest{})
	as.NoError(err)

	// should only have ourself
	as.Len(resp.GetNodes(), 1)
	as.True(assertNodes(resp.GetNodes(), []*protocol.Node{tn}))

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCGetNodes(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, err := generateToken()
	as.NoError(err)
	token := &protocol.ClientToken{
		Token: b,
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)

	node.On("ID").Return(cht.GetId())
	node.On("Identity").Return(cht)

	nodes, vlist := makeNodeList(chord.ExtendedSuccessorEntries)

	pair := &protocol.TunnelDestination{
		Chord:  cht,
		Tunnel: tn,
	}
	pairBuf, err := pair.MarshalVT()
	as.Nil(err)

	node.On("GetSuccessors").Return(vlist, nil)
	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		exp := make([][]byte, len(nodes)+1)
		exp[0] = []byte(tun.DestinationByChordKey(cht))
		for i := 1; i < len(exp); i++ {
			exp[i] = []byte(tun.DestinationByChordKey(nodes[i-1]))
		}
		return assertBytes(k, exp...)
	})).Return(pairBuf, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	callCtx := rpc.WithClientToken(ctx, token)
	resp, err := cRPC.GetNodes(rpc.WithNode(callCtx, cli), &protocol.GetNodesRequest{})

	as.NoError(err)
	as.Len(resp.GetNodes(), tun.NumRedundantLinks)
	as.True(assertNodes(resp.GetNodes(), []*protocol.Node{tn, nodes[0], nodes[1]}))
	as.False(assertNodes(resp.GetNodes(), []*protocol.Node{nodes[2]}))

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCRequestHostnameOK(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, err := generateToken()
	as.NoError(err)
	token := &protocol.ClientToken{
		Token: b,
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)
	node.On("PrefixAppend",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.Anything,
	).Return(nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	callCtx := rpc.WithClientToken(ctx, token)
	resp, err := cRPC.GenerateHostname(rpc.WithNode(callCtx, cli), &protocol.GenerateHostnameRequest{})
	as.NoError(err)

	as.NotEmpty(resp.GetHostname())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCOtherFailed(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node.On("Get", mock.Anything, mock.Anything).Return(nil, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	requests := []*protocol.ClientToken{
		// non-existent token
		{
			Token: []byte("nah"),
		},
	}

	for _, req := range requests {
		callCtx := rpc.WithClientToken(ctx, req)
		resp, err := cRPC.GenerateHostname(rpc.WithNode(callCtx, cli), &protocol.GenerateHostnameRequest{})
		as.Error(err)
		as.Nil(resp)
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)

}

func TestRPCPublishTunnelOK(t *testing.T) {
	as := require.New(t)

	logger, node, _, _, serv := getFixture(t, as)
	cli, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes, _ := makeNodeList(tun.NumRedundantLinks)

	pair := &protocol.TunnelDestination{
		Chord:  cht,
		Tunnel: tn,
	}
	pairBuf, err := pair.MarshalVT()
	as.Nil(err)

	hostname := "test-1234"
	b, err := generateToken()
	as.NoError(err)
	token := &protocol.ClientToken{
		Token: b,
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(k []byte) bool {
			exp := make([][]byte, len(nodes)+1)
			exp[0] = []byte(tun.DestinationByTunnelKey(cht))
			for i := 1; i < len(exp); i++ {
				exp[i] = []byte(tun.DestinationByTunnelKey(nodes[i-1]))
			}
			return assertBytes(k, exp...)
		}),
	).Return(pairBuf, nil)

	fakeLease := uint64(1234)
	node.On("Acquire",
		mock.Anything,
		mock.MatchedBy(func(k []byte) bool {
			return bytes.Equal(k, []byte(tun.ClientLeaseKey(token)))
		}),
		mock.Anything,
	).Return(fakeLease, nil)

	node.On("Release",
		mock.Anything,
		mock.MatchedBy(func(k []byte) bool {
			return bytes.Equal(k, []byte(tun.ClientLeaseKey(token)))
		}),
		fakeLease,
	).Return(nil)

	node.On("Put",
		mock.Anything,
		mock.MatchedBy(func(k []byte) bool {
			return true
		}), mock.MatchedBy(func(v []byte) bool {
			return true
		}),
	).Return(nil)

	node.On("PrefixContains",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.MatchedBy(func(child []byte) bool {
			return bytes.Equal(child, []byte(hostname))
		}),
	).Return(true, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	callCtx := rpc.WithClientToken(ctx, token)
	resp, err := cRPC.PublishTunnel(rpc.WithNode(callCtx, cli), &protocol.PublishTunnelRequest{
		Hostname: hostname,
		Servers:  nodes,
	})

	as.NoError(err)
	as.NotNil(resp)
	as.Len(resp.GetPublished(), len(nodes))

	node.AssertExpectations(t)
}

func TestRPCPublishTunnelFailed(t *testing.T) {
	as := require.New(t)

	logger, node, _, _, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, err := generateToken()
	as.NoError(err)
	token := &protocol.ClientToken{
		Token: b,
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)

	node.On("PrefixContains", mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()

	fakeLease := uint64(1234)
	node.On("Acquire",
		mock.Anything,
		mock.MatchedBy(func(k []byte) bool {
			return bytes.Equal(k, []byte(tun.ClientLeaseKey(token)))
		}),
		mock.Anything,
	).Return(fakeLease, nil).Once()

	node.On("Release",
		mock.Anything,
		mock.MatchedBy(func(k []byte) bool {
			return bytes.Equal(k, []byte(tun.ClientLeaseKey(token)))
		}),
		fakeLease,
	).Return(nil).Once()

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	requests := []struct {
		req   *protocol.PublishTunnelRequest
		token *protocol.ClientToken
	}{
		// missing Token
		{},
		// has token but hostname not requested
		{
			token: token,
			req: &protocol.PublishTunnelRequest{
				Hostname: "nil",
				Servers: []*protocol.Node{
					{
						Id: chord.Random(),
					},
				},
			},
		},
		// has token but not enough servers
		{
			token: token,
			req: &protocol.PublishTunnelRequest{
				Servers: nil,
			},
		},

		// has token but too many servers
		{
			token: token,
			req: &protocol.PublishTunnelRequest{
				Servers: makeNodes(tun.NumRedundantLinks * 2),
			},
		},
	}

	for _, req := range requests {
		callCtx := rpc.WithClientToken(ctx, token)
		resp, err := cRPC.PublishTunnel(rpc.WithNode(callCtx, cli), req.req)
		as.Error(err)
		as.Nil(resp)
	}

	node.AssertExpectations(t)

}
