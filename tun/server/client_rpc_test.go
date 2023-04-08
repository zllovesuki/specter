package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/pki"
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
			Address: strings.Join(generator.MustGenerate(5), "-"),
			Id:      chord.Random(),
		})
	}
	return nodes
}

func makeNodeList(num int) ([]*protocol.Node, []chord.VNode) {
	nodes := make([]*protocol.Node, num)
	list := make([]chord.VNode, num)
	for i := range nodes {
		nodes[i] = &protocol.Node{
			Address: strings.Join(generator.MustGenerate(5), "-"),
			Id:      chord.Random(),
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

func mustGenerateToken() []byte {
	b := make([]byte, 32)
	n, err := io.ReadFull(rand.Reader, b)
	if n != len(b) || err != nil {
		panic(fmt.Errorf("error generating token: %w", err))
	}
	return []byte(base64.StdEncoding.EncodeToString(b))
}

func toCertificate(as *require.Assertions, client *protocol.Node, token *protocol.ClientToken) *x509.Certificate {
	// generate a CA
	caPubKey, caPrivKey, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"dev"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, caPubKey, caPrivKey)
	as.NoError(err)

	certPubKey, _, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	der, err := pki.GenerateCertificate(tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  caPrivKey,
	}, pki.IdentityRequest{
		Subject:   pki.MakeSubjectV1(client.GetId(), string(token.GetToken())),
		PublicKey: certPubKey,
	})
	as.NoError(err)

	cert, err := x509.ParseCertificate(der)
	as.NoError(err)

	return cert
}

func TestRPCRegisterClientNewCertificateOK(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testToken := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	node.On("Put", mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(testToken)))
		}),
		mock.Anything).Return(nil).Once()
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
	tp.WithCertificate(toCertificate(as, cli, testToken))
	resp, err := cRPC.RegisterIdentity(rpc.WithNode(ctx, cli), &protocol.RegisterIdentityRequest{})

	as.NoError(err)
	as.NotNil(resp.GetApex())

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

	testToken := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	node.On("Put", mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(testToken)))
		}),
		mock.Anything).Return(fmt.Errorf("failed")).Once()
	clientT.On("SendDatagram", mock.Anything, mock.Anything).Return(fmt.Errorf("failed")).Once()
	clientT.On("SendDatagram", mock.Anything, mock.Anything).Return(nil).Once()

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	requests := []struct {
		Client *protocol.Node
	}{

		// verified Client but not connected
		{
			Client: cli,
		},

		// verified Client and connected but KV failed
		{
			Client: cli,
		},
	}

	for _, req := range requests {
		tp.WithCertificate(toCertificate(as, req.Client, testToken))
		_, err := cRPC.RegisterIdentity(rpc.WithNode(ctx, cli), &protocol.RegisterIdentityRequest{})
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

	tp.WithCertificate(toCertificate(as, cli, &protocol.ClientToken{}))
	resp, err := cRPC.Ping(rpc.WithNode(ctx, cli), &protocol.ClientPingRequest{})

	as.NoError(err)
	as.Equal(testRootDomain, resp.GetApex())
	as.Equal(tn.GetId(), resp.GetNode().GetId())

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

	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
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

	node.On("Identity").Return(cht)
	node.On("GetSuccessors").Return([]chord.VNode{getVNode(cht)}, nil)
	node.On("Get", mock.Anything, mock.Anything).Return(pairBuf, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	tp.WithCertificate(toCertificate(as, cli, token))
	resp, err := cRPC.GetNodes(rpc.WithNode(ctx, cli), &protocol.GetNodesRequest{})
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

	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)

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

	tp.WithCertificate(toCertificate(as, cli, token))
	resp, err := cRPC.GetNodes(rpc.WithNode(ctx, cli), &protocol.GetNodesRequest{})

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

	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
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

	tp.WithCertificate(toCertificate(as, cli, token))
	resp, err := cRPC.GenerateHostname(rpc.WithNode(ctx, cli), &protocol.GenerateHostnameRequest{})
	as.NoError(err)

	as.NotEmpty(resp.GetHostname())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestRPCRegisteredHostnames(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	hostnames := []string{"hostname-A", "hostname-B"}
	hostnameBytes := make([][]byte, len(hostnames))
	for i, h := range hostnames {
		hostnameBytes[i] = []byte(h)
	}

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)
	node.On("PrefixList",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
	).Return(hostnameBytes, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	tp.WithCertificate(toCertificate(as, cli, token))
	resp, err := cRPC.RegisteredHostnames(rpc.WithNode(ctx, cli), &protocol.RegisteredHostnamesRequest{})
	as.NoError(err)

	as.Len(resp.GetHostnames(), len(hostnames))
	as.EqualValues(hostnames, resp.GetHostnames())

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

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	requests := []*protocol.ClientToken{
		// non-existent token
		{
			Token: []byte("nah"),
		},
	}

	for _, req := range requests {
		tp.WithCertificate(toCertificate(as, cli, req))
		resp, err := cRPC.GenerateHostname(rpc.WithNode(ctx, cli), &protocol.GenerateHostnameRequest{})
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
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
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

	tp.WithCertificate(toCertificate(as, cli, token))
	resp, err := cRPC.PublishTunnel(rpc.WithNode(ctx, cli), &protocol.PublishTunnelRequest{
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

	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
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

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	requests := []*protocol.PublishTunnelRequest{
		// has token but hostname not requested
		{
			Hostname: "nil",
			Servers: []*protocol.Node{
				{
					Id: chord.Random(),
				},
			},
		},
		// has token but not enough servers
		{
			Servers: nil,
		},

		// has token but too many servers
		{
			Servers: makeNodes(tun.NumRedundantLinks * 2),
		},
	}

	for _, req := range requests {
		tp.WithCertificate(toCertificate(as, cli, token))
		resp, err := cRPC.PublishTunnel(rpc.WithNode(ctx, cli), req)
		as.Error(err)
		as.Nil(resp)
	}

	node.AssertExpectations(t)
}

func TestUnpublishTunnel(t *testing.T) {
	as := require.New(t)

	logger, node, _, _, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname := "test-1234"
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	fakeLease := uint64(1234)
	acquireCall := node.On("Acquire",
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
	).Return(nil).NotBefore(acquireCall)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()

	node.On("PrefixContains",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.MatchedBy(func(child []byte) bool {
			return bytes.Equal(child, []byte(hostname))
		}),
	).Return(true, nil)

	for i := 0; i < tun.NumRedundantLinks; i++ {
		key := tun.RoutingKey(hostname, i+1)
		node.On("Delete", mock.Anything, []byte(key)).Return(nil)
	}

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	tp.WithCertificate(toCertificate(as, cli, token))
	resp, err := cRPC.UnpublishTunnel(rpc.WithNode(ctx, cli), &protocol.UnpublishTunnelRequest{
		Hostname: hostname,
	})

	as.NoError(err)
	as.NotNil(resp)

	node.AssertExpectations(t)
}

func TestReleaseTunnel(t *testing.T) {
	as := require.New(t)

	logger, node, _, _, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname := "test-1234"
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	fakeLease := uint64(1234)
	acquireCall := node.On("Acquire",
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
	).Return(nil).NotBefore(acquireCall)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()

	node.On("PrefixContains",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.MatchedBy(func(child []byte) bool {
			return bytes.Equal(child, []byte(hostname))
		}),
	).Return(true, nil)

	deleteCalls := make([]*mock.Call, 0)
	for i := 0; i < tun.NumRedundantLinks; i++ {
		key := tun.RoutingKey(hostname, i+1)
		deleteCall := node.On("Delete", mock.Anything, []byte(key)).Return(nil)
		deleteCalls = append(deleteCalls, deleteCall)
	}

	node.On("PrefixRemove",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.MatchedBy(func(child []byte) bool {
			return bytes.Equal(child, []byte(hostname))
		}),
	).Return(nil).NotBefore(deleteCalls...)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	tp.WithCertificate(toCertificate(as, cli, token))
	resp, err := cRPC.ReleaseTunnel(rpc.WithNode(ctx, cli), &protocol.ReleaseTunnelRequest{
		Hostname: hostname,
	})

	as.NoError(err)
	as.NotNil(resp)

	node.AssertExpectations(t)
}

func TestTokenUpgrade(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	old := &protocol.Node{
		Id: cli.GetId(),
	}
	clientBuf, err := old.MarshalVT()
	as.NoError(err)

	cert := toCertificate(as, cli, token)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)
	node.On("Put",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
		mock.MatchedBy(func(val []byte) bool {
			c := &protocol.Node{}
			err := c.UnmarshalVT(val)
			as.NoError(err)

			t.Log(c)

			return strings.Contains(cert.Subject.CommonName, c.GetAddress()) && c.GetRendezvous()
		}),
	).Return(nil).Once()

	pair := &protocol.TunnelDestination{
		Chord:  cht,
		Tunnel: tn,
	}
	pairBuf, err := pair.MarshalVT()
	as.Nil(err)

	node.On("Identity").Return(cht)
	node.On("GetSuccessors").Return([]chord.VNode{getVNode(cht)}, nil)
	node.On("Get", mock.Anything, mock.Anything).Return(pairBuf, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(ctx, tp)

	tp.WithCertificate(cert)
	_, err = cRPC.GetNodes(rpc.WithNode(ctx, cli), &protocol.GetNodesRequest{})
	as.NoError(err)

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}
