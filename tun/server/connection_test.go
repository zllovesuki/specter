package server

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type negotiationConn struct {
	net.Conn
	writeStarted chan struct{}
	deadline     time.Time
}

func (c *negotiationConn) Write(p []byte) (int, error) {
	select {
	case c.writeStarted <- struct{}{}:
	default:
	}
	return c.Conn.Write(p)
}

func (c *negotiationConn) SetDeadline(deadline time.Time) error {
	c.deadline = deadline
	return c.Conn.SetDeadline(deadline)
}

func connectionFixture(t *testing.T, direct bool, conn net.Conn) (*Server, *protocol.TunnelRoute, *protocol.Link, <-chan context.Context) {
	t.Helper()
	_, _, clientT, chordT, serv := getFixture(t, require.New(t))
	t.Cleanup(serv.routeCache.Close)
	t.Cleanup(serv.keylessCache.Close)
	client, chordNode, tunnelNode := getIdentities()
	link := &protocol.Link{Hostname: "negotiation.example.com", Alpn: protocol.Link_HTTP}
	route := &protocol.TunnelRoute{
		Hostname: link.GetHostname(), ClientDestination: client,
		ChordDestination: chordNode, TunnelDestination: tunnelNode,
	}
	dialContexts := make(chan context.Context, 1)
	recordDialContext := func(args mock.Arguments) { dialContexts <- args.Get(0).(context.Context) }
	if direct {
		clientT.On("Identity").Return(tunnelNode)
		clientT.On("DialStream", mock.Anything, client, protocol.Stream_DIRECT).Return(conn, nil).Once().Run(recordDialContext)
	} else {
		clientT.On("Identity").Return(&protocol.Node{Address: "local-tunnel:123"})
		chordT.On("DialStream", mock.Anything, chordNode, protocol.Stream_PROXY).Return(conn, nil).Once().Run(recordDialContext)
	}
	t.Cleanup(func() {
		clientT.AssertExpectations(t)
		chordT.AssertExpectations(t)
	})
	return serv, route, link, dialContexts
}

func sendProxyStatus(peer net.Conn, status *protocol.TunnelStatus) error {
	if err := rpc.BoundedReceive(peer, &protocol.TunnelRoute{}, 2048); err != nil {
		return err
	}
	return rpc.Send(peer, status)
}

func TestGetConnCancellationInterruptsNegotiationWrites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		direct bool
	}{
		{name: "proxy_route"},
		{name: "direct_link", direct: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, peer := net.Pipe()
			conn := &negotiationConn{
				Conn: local, writeStarted: make(chan struct{}, 1),
			}
			t.Cleanup(func() { conn.Close(); peer.Close() })
			serv, route, link, _ := connectionFixture(t, tc.direct, conn)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := serv.getConn(ctx, route, link)
				done <- err
			}()
			awaitRouteResult(t, conn.writeStarted)
			require.NoError(t, peer.SetReadDeadline(time.Now().Add(time.Second)))
			cancel()
			require.Error(t, awaitRouteResult(t, done))
			_, err := peer.Read(make([]byte, 1))
			require.ErrorIs(t, err, io.EOF, "cancelled negotiation left the stream open")
		})
	}
}

func TestGetConnClosesRejectedProxyStreams(t *testing.T) {
	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })
	serv, route, link, _ := connectionFixture(t, false, local)
	statusSent := make(chan error, 1)
	go func() {
		statusSent <- sendProxyStatus(peer, &protocol.TunnelStatus{Status: protocol.TunnelStatusCode_NO_DIRECT})
	}()
	require.NoError(t, peer.SetReadDeadline(time.Now().Add(time.Second)))
	got, err := serv.getConn(t.Context(), route, link)
	require.Nil(t, got)
	require.ErrorIs(t, err, tun.ErrTunnelClientNotConnected)
	require.NoError(t, awaitRouteResult(t, statusSent))
	_, err = peer.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF, "rejected proxy negotiation left the stream open")
}

func TestGetConnClearsNegotiationDeadline(t *testing.T) {
	for _, direct := range []bool{true, false} {
		name := "proxy"
		if direct {
			name = "direct"
		}
		t.Run(name, func(t *testing.T) {
			local, peer := net.Pipe()
			conn := &negotiationConn{Conn: local}
			t.Cleanup(func() { conn.Close(); peer.Close() })
			serv, route, link, dialContexts := connectionFixture(t, direct, conn)
			peerDone := make(chan error, 1)
			go func() {
				if !direct {
					if err := sendProxyStatus(peer, &protocol.TunnelStatus{}); err != nil {
						peerDone <- err
						return
					}
				}
				if err := rpc.BoundedReceive(peer, &protocol.Link{}, 1024); err != nil {
					peerDone <- err
					return
				}
				_, err := io.ReadFull(peer, make([]byte, 1))
				peerDone <- err
			}()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			got, err := serv.getConn(ctx, route, link)
			require.NoError(t, err)
			require.Same(t, conn, got)
			require.NoError(t, (<-dialContexts).Err(), "completed negotiation cancelled the transport connection context")
			require.True(t, conn.deadline.IsZero(), "successful stream retained its negotiation deadline")
			cancel()
			require.NoError(t, got.SetWriteDeadline(time.Now().Add(time.Second)))
			_, err = got.Write([]byte{1})
			require.NoError(t, err, "completed negotiation retained its cancellation hook")
			require.NoError(t, <-peerDone)
		})
	}
}
