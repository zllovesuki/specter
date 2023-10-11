package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	mocks "go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

const (
	testRootDomain = "hello.com"
	testAcmeZone   = "acme.example.com"
)

func assertBytes(got []byte, exp ...[]byte) bool {
	for _, b := range exp {
		r := bytes.Compare(got, b)
		if r == 0 {
			return true
		}
	}
	return false
}

type configOption func(*Config)

func withResolver(r tun.DNSResolver) configOption {
	return func(c *Config) {
		c.Resolver = r
	}
}

func getFixture(t *testing.T, as *require.Assertions, options ...configOption) (*zap.Logger, *mocks.VNode, *mocks.Transport, *mocks.Transport, *Server) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	n := new(mocks.VNode)
	cht := new(mocks.Transport)
	clt := new(mocks.Transport)

	cfg := Config{
		ParentContext:   context.Background(),
		Logger:          logger,
		Chord:           n,
		TunnelTransport: clt,
		ChordTransport:  cht,
		Apex:            testRootDomain,
		Acme:            testAcmeZone,
	}

	for _, opt := range options {
		opt(&cfg)
	}

	s := New(cfg)

	return logger, n, clt, cht, s
}

func getExpected(link *protocol.Link) [][]byte {
	return [][]byte{
		[]byte(tun.RoutingKey(link.GetHostname(), 1)),
		[]byte(tun.RoutingKey(link.GetHostname(), 2)),
		[]byte(tun.RoutingKey(link.GetHostname(), 3)),
	}
}

func getIdentities() (*protocol.Node, *protocol.Node, *protocol.Node) {
	cl := &protocol.Node{
		Address:    "client:123",
		Id:         chord.Random(),
		Rendezvous: true,
	}
	ch := &protocol.Node{
		Address: "chord:123",
		Id:      chord.Random(),
	}
	tn := &protocol.Node{
		Address: "tunnel:123",
		Id:      chord.Random(),
	}
	return cl, ch, tn
}

func TestContinueLookupOnError(t *testing.T) {
	as := require.New(t)

	_, node, _, _, serv := getFixture(t, as)

	link := &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: "test",
	}
	expected := getExpected(link)

	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		return assertBytes(k, expected...)
	})).Return(nil, fmt.Errorf("panic"))

	c, err := serv.DialClient(context.Background(), link)
	as.ErrorIs(err, tun.ErrLookupFailed)
	as.Nil(c)

	node.AssertExpectations(t)
}

func TestLookupSuccessDirect(t *testing.T) {
	as := require.New(t)

	_, node, clientT, _, serv := getFixture(t, as)

	link := &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: "test",
	}

	cli, cht, tn := getIdentities()
	bundle := &protocol.TunnelRoute{
		ClientDestination: cli,
		ChordDestination:  cht,
		TunnelDestination: tn,
		Hostname:          link.GetHostname(),
	}
	bundleBuf, err := bundle.MarshalVT()
	as.NoError(err)

	expected := getExpected(link)

	// 1. first query the chord network
	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		return assertBytes(k, expected...)
	})).Return(bundleBuf, nil)

	// 2. then it should compare the content in bundle
	clientT.On("Identity").Return(tn)

	// 3. once we figure out that it is connected to us,
	// attempt to dial
	c1, c2 := net.Pipe()
	go func() {
		l := &protocol.Link{}
		err := rpc.Receive(c2, l)
		as.NoError(err)
		as.Equal(link.GetAlpn(), l.GetAlpn())
		as.Equal(link.GetHostname(), l.GetHostname())
	}()
	clientT.On("DialStream", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == cli.GetId()
	}), protocol.Stream_DIRECT).Return(c1, nil)

	_, err = serv.DialClient(context.Background(), link)
	as.NoError(err)

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestLookupSuccessRemote(t *testing.T) {
	as := require.New(t)

	_, node, clientT, chordT, serv := getFixture(t, as)

	link := &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: "test",
	}

	cli, cht, tn := getIdentities()
	bundle := &protocol.TunnelRoute{
		ClientDestination: cli,
		ChordDestination:  cht,
		TunnelDestination: tn,
		Hostname:          link.GetHostname(),
	}
	bundleBuf, err := bundle.MarshalVT()
	as.NoError(err)

	expected := getExpected(link)

	// 1. first query the chord network
	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		return assertBytes(k, expected...)
	})).Return(bundleBuf, nil)

	// 2. then it should compare the content in bundle
	clientT.On("Identity").Return(nil)

	// 3. once we figure out that it is NOT connected to us,
	// attempt to dial via chord
	c1, c2 := net.Pipe()
	go func() {
		// the remote node should receive the bundle
		bundle := &protocol.TunnelRoute{}
		err := rpc.Receive(c2, bundle)
		as.NoError(err)

		// remote node need to send feedback
		tun.SendStatusProto(c2, nil)

		// then receive the link information
		l := &protocol.Link{}
		err = rpc.Receive(c2, l)
		as.NoError(err)
		as.Equal(link.GetAlpn(), l.GetAlpn())
		as.Equal(link.GetHostname(), l.GetHostname())
	}()
	chordT.On("DialStream", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == cht.GetId()
	}), protocol.Stream_PROXY).Return(c1, nil)

	_, err = serv.DialClient(context.Background(), link)
	as.NoError(err)

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestHandleRemoteConnection(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, chordT, serv := getFixture(t, as)
	cli, cht, tn := getIdentities()
	bundle := &protocol.TunnelRoute{
		ClientDestination: cli,
		ChordDestination:  cht,
		TunnelDestination: tn,
		Hostname:          "test",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	syncA := make(chan struct{})
	syncB := make(chan struct{})

	chordChan := make(chan *transport.StreamDelegate)
	chordT.On("AcceptStream").Return(chordChan)
	chordT.On("Identity").Return(cht)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("AcceptStream").Return(clientChan)
	clientT.On("Identity").Return(tn)

	// on start up (Accept), identities should get published
	node.On("Put", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		exp := [][]byte{
			[]byte(tun.DestinationByChordKey(cht)),
			[]byte(tun.DestinationByTunnelKey(tn)),
		}
		return assertBytes(k, exp...)
	}), mock.MatchedBy(func(v []byte) bool {
		pair := &protocol.TunnelDestination{
			Chord:  cht,
			Tunnel: tn,
		}
		buf, err := pair.MarshalVT()
		if err != nil {
			return false
		}
		return assertBytes(v, buf)
	})).Return(nil)

	streamRouter := transport.NewStreamRouter(logger, chordT, clientT)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	// since the "client" is connected to us, we should expect a DialDirect
	// to the client
	c1, c2 := net.Pipe()
	c3, c4 := net.Pipe()
	clientT.On("DialStream", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == cli.GetId()
	}), protocol.Stream_DIRECT).Return(c3, nil)

	buf := []byte{1, 2, 3}

	go func() {
		// the remote should be sending the bundle over
		err := rpc.Send(c2, bundle)
		as.NoError(err)

		// getConn should check the status
		x := &protocol.TunnelStatus{}
		err = rpc.Receive(c2, x)
		as.NoError(err)
		as.Equal(protocol.TunnelStatusCode_STATUS_OK, x.GetStatus())

		// now the remote gateway is sending data to us
		_, err = c2.Write(buf)
		as.NoError(err)

		close(syncA)
	}()

	go func() {
		<-syncA

		// we should receive data from the remote side
		b := make([]byte, len(buf))
		_, err := c4.Read(b)
		as.NoError(err)
		as.EqualValues(buf, b)

		close(syncB)
	}()

	serv.MustRegister(ctx)

	chordChan <- &transport.StreamDelegate{
		Conn:     c1,
		Identity: &protocol.Node{Id: chord.Random()},
		Kind:     protocol.Stream_PROXY,
	}

	select {
	case <-syncB:
	case <-time.After(time.Second * 5):
		as.FailNow("timeout")
	}

	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
	node.AssertExpectations(t)
}
