package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/zllovesuki/specter/rpc"
	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/transport"
	"github.com/zllovesuki/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

func getFixture(as *require.Assertions) (*zap.Logger, *mockVNode, *mockTransport, *mockTransport, *Server) {
	logger, err := zap.NewDevelopment()
	as.Nil(err)

	n := new(mockVNode)
	cht := new(mockTransport)
	clt := new(mockTransport)

	s := New(logger, n, clt, cht, "hello")

	return logger, n, clt, cht, s
}

func getExpected(link *protocol.Link) [][]byte {
	return [][]byte{
		[]byte(tun.BundleKey(link.GetHostname(), 1)),
		[]byte(tun.BundleKey(link.GetHostname(), 2)),
		[]byte(tun.BundleKey(link.GetHostname(), 3)),
	}
}

func getIdentities() (*protocol.Node, *protocol.Node, *protocol.Node) {
	cl := &protocol.Node{
		Id: chord.Random(),
	}
	ch := &protocol.Node{
		Id: chord.Random(),
	}
	tn := &protocol.Node{
		Id: chord.Random(),
	}
	return cl, ch, tn
}

func TestContinueLookupOnError(t *testing.T) {
	as := require.New(t)

	_, node, _, _, serv := getFixture(as)

	link := &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: "test",
	}
	expected := getExpected(link)

	node.On("Get", mock.MatchedBy(func(k []byte) bool {
		return assertBytes(k, expected...)
	})).Return(nil, fmt.Errorf("panic"))

	c, err := serv.Dial(context.Background(), link)
	as.Error(err)
	as.Nil(c)

	node.AssertExpectations(t)
}

func TestLookupSuccessDirect(t *testing.T) {
	as := require.New(t)

	_, node, clientT, _, serv := getFixture(as)

	link := &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: "test",
	}

	cli, cht, tn := getIdentities()
	bundle := &protocol.Tunnel{
		Client:   cli,
		Chord:    cht,
		Tun:      tn,
		Hostname: link.GetHostname(),
	}
	bundleBuf, err := bundle.MarshalVT()
	as.Nil(err)

	expected := getExpected(link)

	// 1. first query the chord network
	node.On("Get", mock.MatchedBy(func(k []byte) bool {
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
		as.Nil(err)
		as.Equal(link.GetAlpn(), l.GetAlpn())
		as.Equal(link.GetHostname(), l.GetHostname())
	}()
	clientT.On("DialDirect", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == cli.GetId()
	})).Return(c1, nil)

	_, err = serv.Dial(context.Background(), link)
	as.Nil(err)

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestLookupSuccessRemote(t *testing.T) {
	as := require.New(t)

	_, node, clientT, chordT, serv := getFixture(as)

	link := &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: "test",
	}

	cli, cht, tn := getIdentities()
	bundle := &protocol.Tunnel{
		Client:   cli,
		Chord:    cht,
		Tun:      tn,
		Hostname: link.GetHostname(),
	}
	bundleBuf, err := bundle.MarshalVT()
	as.Nil(err)

	expected := getExpected(link)

	// 1. first query the chord network
	node.On("Get", mock.MatchedBy(func(k []byte) bool {
		return assertBytes(k, expected...)
	})).Return(bundleBuf, nil)

	// 2. then it should compare the content in bundle
	clientT.On("Identity").Return(nil)

	// 3. once we figure out that it is NOT connected to us,
	// attempt to dial via chord
	c1, c2 := net.Pipe()
	go func() {
		// the remote node should receive the bundle
		bundle := &protocol.Tunnel{}
		err := rpc.Receive(c2, bundle)
		as.Nil(err)

		// then receive the link information
		l := &protocol.Link{}
		err = rpc.Receive(c2, l)
		as.Nil(err)
		as.Equal(link.GetAlpn(), l.GetAlpn())
		as.Equal(link.GetHostname(), l.GetHostname())
	}()
	chordT.On("DialDirect", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == cht.GetId()
	})).Return(c1, nil)

	_, err = serv.Dial(context.Background(), link)
	as.Nil(err)

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}

func TestHandleRemoteConnection(t *testing.T) {
	as := require.New(t)

	_, node, clientT, chordT, serv := getFixture(as)
	cli, cht, tn := getIdentities()
	bundle := &protocol.Tunnel{
		Client:   cli,
		Chord:    cht,
		Tun:      tn,
		Hostname: "test",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sync := make(chan struct{})

	chordChan := make(chan *transport.StreamDelegate)
	chordT.On("Direct").Return(chordChan)
	chordT.On("Identity").Return(cht)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("Direct").Return(clientChan)
	clientT.On("Identity").Return(tn)

	// we really don't care about the dentities put on chord DHT for now
	node.On("Put", mock.Anything, mock.Anything).Return(nil)

	go serv.Accept(ctx)

	c1, c2 := net.Pipe()
	remote := &protocol.Node{Id: chord.Random()}
	del := &transport.StreamDelegate{
		Connection: c1,
		Identity:   remote,
	}

	chordChan <- del

	// since the "client" is connected to us, we should expect a DialDirect
	// to the client
	c3, c4 := net.Pipe()
	clientT.On("DialDirect", mock.Anything, mock.MatchedBy(func(n *protocol.Node) bool {
		return n.GetId() == cli.GetId()
	})).Return(c3, nil)

	buf := []byte{1, 2, 3}

	go func() {
		// the remote should be sending the bundle over
		err := rpc.Send(c2, bundle)
		as.Nil(err)

		// now the remote gateway is sending data to us
		_, err = c2.Write(buf)
		as.Nil(err)
	}()

	go func() {
		// we should receive data from the remote side
		b := make([]byte, len(buf))
		_, err := c4.Read(b)
		as.Nil(err)
		as.EqualValues(buf, b)

		close(sync)
	}()

	select {
	case <-sync:
	case <-time.After(time.Second):
		as.FailNow("timeout")
	}

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}
