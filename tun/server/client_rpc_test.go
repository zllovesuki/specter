package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func makeNodeList() ([]*protocol.Node, []chord.VNode) {
	nodes := make([]*protocol.Node, chord.ExtendedSuccessorEntries)
	list := make([]chord.VNode, chord.ExtendedSuccessorEntries)
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

	logger, node, clientT, chordT, serv := getFixture(as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node.On("Put", mock.Anything, mock.Anything).Return(nil).Once()

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)
	clientT.On("SendDatagram",
		mock.MatchedBy(func(node *protocol.Node) bool {
			return node.GetId() == cli.GetId()
		}),
		mock.MatchedBy(func(b []byte) bool {
			return bytes.Equal(b, []byte(testDatagramData))
		}),
	).Return(nil).Once()

	c1, c2 := net.Pipe()

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind: protocol.TunnelRPC_IDENTITY,
			RegisterRequest: &protocol.RegisterIdentityRequest{
				Client: cli,
			},
		},
	})
	as.NoError(err)
	tResp := resp.GetClientResponse()
	as.NotNil(tResp.GetRegisterResponse())
	as.NotNil(tResp.GetRegisterResponse().GetToken())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCRegisterClientFailed(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node.On("Put", mock.Anything, mock.Anything).Return(fmt.Errorf("failed")).Once()

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)
	clientT.On("SendDatagram", mock.Anything, mock.Anything).Return(fmt.Errorf("failed")).Once()
	clientT.On("SendDatagram", mock.Anything, mock.Anything).Return(nil).Once()

	c1, c2 := net.Pipe()

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	requests := []*protocol.ClientRequest{
		// missing Client
		{
			Kind: protocol.TunnelRPC_IDENTITY,
			RegisterRequest: &protocol.RegisterIdentityRequest{
				Client: nil,
			},
		},

		// has Client but does not match the connected client
		{
			Kind: protocol.TunnelRPC_IDENTITY,
			RegisterRequest: &protocol.RegisterIdentityRequest{
				Client: &protocol.Node{
					Id: chord.Random(),
				},
			},
		},

		// has Client but not connected
		{
			Kind: protocol.TunnelRPC_IDENTITY,
			RegisterRequest: &protocol.RegisterIdentityRequest{
				Client: cli,
			},
		},

		// has Client and connected but KV failed
		{
			Kind: protocol.TunnelRPC_IDENTITY,
			RegisterRequest: &protocol.RegisterIdentityRequest{
				Client: cli,
			},
		},
	}

	for _, req := range requests {
		resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
			Kind:          protocol.RPC_CLIENT_REQUEST,
			ClientRequest: req,
		})
		as.Error(err)
		tResp := resp.GetClientResponse()
		as.Nil(tResp.GetRegisterResponse())
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCPingOK(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
	cli, _, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)
	clientT.On("Identity").Return(tn)

	c1, c2 := net.Pipe()

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind: protocol.TunnelRPC_PING,
		},
	})
	as.NoError(err)
	as.NotNil(resp.GetClientResponse())
	as.NotNil(resp.GetClientResponse().GetPingResponse())

	r := resp.GetClientResponse().GetPingResponse()
	as.Equal(testRootDomain, r.GetApex())
	as.Equal(tn.GetId(), r.GetNode().GetId())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCPingConditional(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
	cli, _, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get", mock.Anything, mock.Anything).Return(clientBuf, nil).Once()
	node.On("Get", mock.Anything, mock.Anything).Return(nil, nil).Once()

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)
	clientT.On("Identity").Return(tn)

	c1, c2 := net.Pipe()

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	tests := []struct {
		req         *protocol.ClientRequest
		expectError bool
	}{
		{
			req: &protocol.ClientRequest{
				Kind: protocol.TunnelRPC_PING,
			},
			expectError: false,
		},
		{
			req: &protocol.ClientRequest{
				Kind: protocol.TunnelRPC_PING,
				Token: &protocol.ClientToken{
					Token: []byte("correct"),
				},
			},
			expectError: false,
		},
		{
			req: &protocol.ClientRequest{
				Kind: protocol.TunnelRPC_PING,
				Token: &protocol.ClientToken{
					Token: []byte("incorrect"),
				},
			},
			expectError: true,
		},
	}

	for _, req := range tests {
		resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
			Kind:          protocol.RPC_CLIENT_REQUEST,
			ClientRequest: req.req,
		})
		if req.expectError {
			as.Error(err)
		} else {
			as.NoError(err)
			as.NotNil(resp.GetClientResponse())
			as.Equal(testRootDomain, resp.GetClientResponse().GetPingResponse().GetApex())
		}
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCGetNodesUnique(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
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
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)

	node.On("ID").Return(cht.GetId())
	node.On("Identity").Return(cht)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)

	c1, c2 := net.Pipe()

	pair := &protocol.IdentitiesPair{
		Chord: cht,
		Tun:   tn,
	}
	pairBuf, err := pair.MarshalVT()
	as.Nil(err)

	node.On("GetSuccessors").Return([]chord.VNode{getVNode(cht)}, nil)
	node.On("Get", mock.Anything).Return(pairBuf, nil)

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:  protocol.TunnelRPC_NODES,
			Token: token,
		},
	})
	as.NoError(err)
	as.NotNil(resp.ClientResponse)
	as.NotNil(resp.ClientResponse.NodesResponse)

	// should only have ourself
	as.Len(resp.GetClientResponse().GetNodesResponse().GetNodes(), 1)
	as.True(assertNodes(resp.GetClientResponse().GetNodesResponse().GetNodes(), []*protocol.Node{tn}))

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCGetNodes(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
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
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)

	node.On("ID").Return(cht.GetId())
	node.On("Identity").Return(cht)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)

	c1, c2 := net.Pipe()

	nodes, vlist := makeNodeList()

	pair := &protocol.IdentitiesPair{
		Chord: cht,
		Tun:   tn,
	}
	pairBuf, err := pair.MarshalVT()
	as.Nil(err)

	node.On("GetSuccessors").Return(vlist, nil)
	node.On("Get", mock.MatchedBy(func(k []byte) bool {
		exp := make([][]byte, len(nodes)+1)
		exp[0] = []byte(tun.IdentitiesChordKey(cht))
		for i := 1; i < len(exp); i++ {
			exp[i] = []byte(tun.IdentitiesChordKey(nodes[i-1]))
		}
		return assertBytes(k, exp...)
	})).Return(pairBuf, nil)

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:  protocol.TunnelRPC_NODES,
			Token: token,
		},
	})
	as.NoError(err)
	as.NotNil(resp.ClientResponse)
	as.NotNil(resp.ClientResponse.NodesResponse)

	as.Len(resp.GetClientResponse().GetNodesResponse().GetNodes(), tun.NumRedundantLinks)
	as.True(assertNodes(resp.GetClientResponse().GetNodesResponse().GetNodes(), []*protocol.Node{tn, nodes[0], nodes[1]}))
	as.False(assertNodes(resp.GetClientResponse().GetNodesResponse().GetNodes(), []*protocol.Node{nodes[2]}))

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCRequestHostnameOK(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
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
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)
	node.On("PrefixAppend",
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.Anything,
	).Return(nil)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)

	c1, c2 := net.Pipe()

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:            protocol.TunnelRPC_HOSTNAME,
			Token:           token,
			HostnameRequest: &protocol.GenerateHostnameRequest{},
		},
	})
	as.NoError(err)
	tResp := resp.GetClientResponse()
	as.NotNil(tResp.GetHostnameResponse())
	as.NotEmpty(tResp.GetHostnameResponse().GetHostname())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCOtherFailed(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node.On("Get", mock.Anything).Return(nil, nil)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)

	c1, c2 := net.Pipe()

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	requests := []*protocol.ClientRequest{
		// non-existent token
		{
			Kind: protocol.TunnelRPC_HOSTNAME,
			Token: &protocol.ClientToken{
				Token: []byte("nah"),
			},
			HostnameRequest: &protocol.GenerateHostnameRequest{},
		},
	}

	for _, req := range requests {
		resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
			Kind:          protocol.RPC_CLIENT_REQUEST,
			ClientRequest: req,
		})
		as.Error(err)
		tResp := resp.GetClientResponse()
		as.Nil(tResp.GetRegisterResponse())
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)

}

func TestRPCPublishTunnelOK(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
	cli, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)

	c1, c2 := net.Pipe()

	nodes, _ := makeNodeList()

	pair := &protocol.IdentitiesPair{
		Chord: cht,
		Tun:   tn,
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
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()

	node.On("Get",
		mock.MatchedBy(func(k []byte) bool {
			exp := make([][]byte, len(nodes)+1)
			exp[0] = []byte(tun.IdentitiesTunKey(cht))
			for i := 1; i < len(exp); i++ {
				exp[i] = []byte(tun.IdentitiesTunKey(nodes[i-1]))
			}
			return assertBytes(k, exp...)
		}),
	).Return(pairBuf, nil)

	node.On("Put",
		mock.MatchedBy(func(k []byte) bool {
			return true
		}), mock.MatchedBy(func(v []byte) bool {
			return true
		}),
	).Return(nil)

	node.On("PrefixContains",
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.MatchedBy(func(child []byte) bool {
			return bytes.Equal(child, []byte(hostname))
		}),
	).Return(true, nil)

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:  protocol.TunnelRPC_TUNNEL,
			Token: token,
			TunnelRequest: &protocol.PublishTunnelRequest{
				Hostname: hostname,
				Servers:  nodes,
			},
		},
	})
	as.NoError(err)
	as.NotNil(resp.GetClientResponse())
	tResp := resp.GetClientResponse()
	as.NotNil(tResp.GetTunnelResponse())
	as.Len(tResp.GetTunnelResponse().GetPublished(), len(nodes))

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCPublishTunnelFailed(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
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
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)

	node.On("PrefixContains", mock.Anything, mock.Anything).Return(false, nil).Once()

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("RPC").Return(clientChan)

	c1, c2 := net.Pipe()

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	requests := []*protocol.ClientRequest{
		// missing Token
		{
			Kind:          protocol.TunnelRPC_TUNNEL,
			TunnelRequest: &protocol.PublishTunnelRequest{},
		},

		// has token but hostname not requested
		{
			Kind:  protocol.TunnelRPC_TUNNEL,
			Token: token,
			TunnelRequest: &protocol.PublishTunnelRequest{
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
			Kind:  protocol.TunnelRPC_TUNNEL,
			Token: token,
			TunnelRequest: &protocol.PublishTunnelRequest{
				Servers: nil,
			},
		},

		// has token but too many servers
		{
			Kind:  protocol.TunnelRPC_TUNNEL,
			Token: token,
			TunnelRequest: &protocol.PublishTunnelRequest{
				Servers: makeNodes(tun.NumRedundantLinks * 2),
			},
		},
	}

	for _, req := range requests {
		resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
			Kind:          protocol.RPC_CLIENT_REQUEST,
			ClientRequest: req,
		})
		as.Error(err)
		tResp := resp.GetClientResponse()
		as.Nil(tResp.GetRegisterResponse())
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)

}
