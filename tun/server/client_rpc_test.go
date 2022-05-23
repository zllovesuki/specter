package server

import (
	"context"
	"net"
	"testing"

	"github.com/zllovesuki/specter/rpc"
	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/transport"
	"github.com/zllovesuki/specter/spec/tun"

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

func TestRPCGetNodes(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(as)
	cli, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		Kind: protocol.RPC_GET_NODES,
	})
	as.Nil(err)
	as.NotNil(resp.GetNodesResponse)

	as.Len(resp.GetGetNodesResponse().GetNodes(), tun.NumRedundantLinks)
	as.True(assertNodes(resp.GetGetNodesResponse().GetNodes(), []*protocol.Node{tn}))

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCPublishTunnel(t *testing.T) {
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

	node.On("Get", mock.MatchedBy(func(k []byte) bool {
		exp := make([][]byte, len(nodes)+1)
		exp[0] = []byte(tun.IdentitiesTunKey(cht))
		for i := 1; i < len(exp); i++ {
			exp[i] = []byte(tun.IdentitiesTunKey(nodes[i-1]))
		}
		return assertBytes(k, exp...)
	})).Return(pairBuf, nil)

	node.On("Put", mock.MatchedBy(func(k []byte) bool {
		return true
	}), mock.MatchedBy(func(v []byte) bool {
		return true
	})).Return(nil)

	go serv.HandleRPC(ctx)

	clientChan <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   cli,
	}

	cRPC := rpc.NewRPC(logger, c2, nil)
	go cRPC.Start(ctx)

	resp, err := cRPC.Call(ctx, &protocol.RPC_Request{
		Kind: protocol.RPC_PUBLISH_TUNNEL,
		PublishTunnelRequest: &protocol.PublishTunnelRequest{
			Client:  cli,
			Servers: nodes[1:],
		},
	})
	as.Nil(err)
	as.NotNil(resp.PublishTunnelResponse)
	as.NotEmpty(resp.GetPublishTunnelResponse().GetHostname())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}
