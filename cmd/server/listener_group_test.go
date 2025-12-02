package server

import (
	"net"
	"testing"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	cmdlisten "go.miragespace.co/specter/cmd/internal/listen"
)

type stubAddr string

func (a stubAddr) Network() string { return "stub" }
func (a stubAddr) String() string  { return string(a) }

type stubListener struct {
	conns  chan net.Conn
	closed chan struct{}
	addr   net.Addr
}

func newStubListener(name string) *stubListener {
	return &stubListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
		addr:   stubAddr(name),
	}
}

func (l *stubListener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-l.conns:
		if !ok {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *stubListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
		close(l.conns)
	}
	return nil
}

func (l *stubListener) Addr() net.Addr { return l.addr }

func TestMultiListenerAcceptsAcrossListeners(t *testing.T) {
	l1 := newStubListener("l1")
	l2 := newStubListener("l2")
	ml := newMultiListener([]net.Listener{l1, l2})
	defer ml.Close()

	serverConn1, clientConn1 := net.Pipe()
	serverConn2, clientConn2 := net.Pipe()
	defer clientConn1.Close()
	defer clientConn2.Close()

	l1.conns <- serverConn1
	l2.conns <- serverConn2

	got := make(map[net.Conn]struct{})
	for i := 0; i < 2; i++ {
		conn, err := ml.Accept()
		require.NoError(t, err)
		got[conn] = struct{}{}
	}

	_, ok := got[serverConn1]
	require.True(t, ok, "expected connection from first listener")
	_, ok = got[serverConn2]
	require.True(t, ok, "expected connection from second listener")

	require.NoError(t, ml.Close())
	_, err := ml.Accept()
	require.ErrorIs(t, err, net.ErrClosed)
}

func TestMultiDialerPickPrefersMatchingIPVersion(t *testing.T) {
	v4 := &quic.Transport{}
	v6 := &quic.Transport{}
	any := &quic.Transport{}

	bindings := []udpBinding{
		{listen: cmdlisten.Address{Version: cmdlisten.IPV4}, transport: v4},
		{listen: cmdlisten.Address{Version: cmdlisten.IPV6}, transport: v6},
		{listen: cmdlisten.Address{Version: cmdlisten.IPAny}, transport: any},
	}

	md := newMultiDialer(bindings)

	require.Equal(t, v4, md.pick(cmdlisten.IPV4))
	require.Equal(t, v6, md.pick(cmdlisten.IPV6))
	require.Equal(t, v4, md.pick(cmdlisten.IPAny))
}
