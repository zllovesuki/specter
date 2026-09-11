package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/bufconn"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDialClientRefreshesExhaustedRoutes(t *testing.T) {
	as := require.New(t)
	_, node, clientT, _, serv := getFixture(t, as)
	t.Cleanup(serv.routeCache.Close)
	t.Cleanup(serv.keylessCache.Close)
	link := &protocol.Link{Hostname: "recovered.example.com", Alpn: protocol.Link_HTTP}
	oldClient, chordNode, tunnelNode := getIdentities()
	newClient := &protocol.Node{Address: "new-client:123", Id: oldClient.GetId() + 1}
	clientT.On("Identity").Return(tunnelNode)

	for _, client := range []*protocol.Node{oldClient, newClient} {
		route := &protocol.TunnelRoute{
			Hostname: link.GetHostname(), ClientDestination: client,
			ChordDestination: chordNode, TunnelDestination: tunnelNode,
		}
		wire, err := route.MarshalVT()
		as.NoError(err)
		node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), 1))).Return(wire, nil).Once()
		for i := 2; i <= tun.NumRedundantLinks; i++ {
			node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), i))).Return(nil, nil).Once()
		}
	}
	serv.RoutesPreload(link.GetHostname())
	clientT.On("DialStream", mock.Anything, oldClient, protocol.Stream_DIRECT).Return(nil, transport.ErrNoDirect).Once()
	client, remote := bufconn.BufferedPipe(8192)
	t.Cleanup(func() { client.Close(); remote.Close() })
	clientT.On("DialStream", mock.Anything, newClient, protocol.Stream_DIRECT).Return(client, nil).Once()

	conn, err := serv.DialClient(t.Context(), link)
	as.NoError(err)
	as.Same(client, conn)
	received := &protocol.Link{}
	as.NoError(rpc.BoundedReceive(remote, received, 1024))
	as.Equal(link.GetHostname(), received.GetHostname())
	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestDialClientKeepsUsableFallback(t *testing.T) {
	as := require.New(t)
	_, node, clientT, _, serv := getFixture(t, as)
	t.Cleanup(serv.routeCache.Close)
	t.Cleanup(serv.keylessCache.Close)
	link := &protocol.Link{Hostname: "fallback.example.com"}
	client, chordNode, tunnelNode := getIdentities()
	fallback := &protocol.Node{Address: "fallback:123", Id: client.GetId() + 1}
	clientT.On("Identity").Return(tunnelNode)
	for i, destination := range []*protocol.Node{client, fallback} {
		wire, err := (&protocol.TunnelRoute{
			Hostname: link.GetHostname(), ClientDestination: destination,
			ChordDestination: chordNode, TunnelDestination: tunnelNode,
		}).MarshalVT()
		as.NoError(err)
		node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), i+1))).Return(wire, nil).Once()
	}
	node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), 3))).Return(nil, errors.New("temporary lookup failure")).Once()
	clientT.On("DialStream", mock.Anything, client, protocol.Stream_DIRECT).Return(nil, transport.ErrNoDirect).Once()
	conn, remote := bufconn.BufferedPipe(8192)
	t.Cleanup(func() { conn.Close(); remote.Close() })
	clientT.On("DialStream", mock.Anything, fallback, protocol.Stream_DIRECT).Return(conn, nil).Once()

	got, err := serv.DialClient(t.Context(), link)
	as.NoError(err)
	as.Same(conn, got)
	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestDialClientRefreshCooldown(t *testing.T) {
	as := require.New(t)
	_, node, clientT, _, serv := getFixture(t, as)
	t.Cleanup(serv.routeCache.Close)
	t.Cleanup(serv.keylessCache.Close)
	link := &protocol.Link{Hostname: "still-offline.example.com"}
	client, chordNode, tunnelNode := getIdentities()
	route := &protocol.TunnelRoute{
		Hostname: link.GetHostname(), ClientDestination: client,
		ChordDestination: chordNode, TunnelDestination: tunnelNode,
	}
	as.True(serv.routeCache.SetWithTTL(link.GetHostname(), &routesResult{routes: []*protocol.TunnelRoute{route}}, 128, routePositiveTTL))
	wire, err := route.MarshalVT()
	as.NoError(err)
	// The first and third requests refresh; the second is inside the cooldown.
	node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), 1))).Return(wire, nil).Twice()
	for i := 2; i <= tun.NumRedundantLinks; i++ {
		node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), i))).Return(nil, nil).Twice()
	}
	clientT.On("Identity").Return(tunnelNode)
	clientT.On("DialStream", mock.Anything, client, protocol.Stream_DIRECT).Return(nil, transport.ErrNoDirect).Times(5)
	for i := 0; i < 2; i++ {
		conn, err := serv.DialClient(t.Context(), link)
		as.Nil(conn)
		as.ErrorIs(err, tun.ErrTunnelClientNotConnected)
	}
	cached, ok := serv.routeCache.Get(link.GetHostname())
	as.True(ok)
	expired := *cached
	expired.refreshAfter = time.Now().Add(-time.Second)
	as.True(serv.routeCache.SetWithTTL(link.GetHostname(), &expired, 128, routePositiveTTL))
	conn, err := serv.DialClient(t.Context(), link)
	as.Nil(conn)
	as.ErrorIs(err, tun.ErrTunnelClientNotConnected)
	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestDialClientSharesRefreshWithConcurrentAndDelayedFailures(t *testing.T) {
	as := require.New(t)
	_, node, clientT, _, serv := getFixture(t, as)
	t.Cleanup(serv.routeCache.Close)
	t.Cleanup(serv.keylessCache.Close)
	link := &protocol.Link{Hostname: "concurrent.example.com"}
	oldClient, chordNode, tunnelNode := getIdentities()
	newClient := &protocol.Node{Address: "new-client:123", Id: oldClient.GetId() + 1}
	route := &protocol.TunnelRoute{
		Hostname: link.GetHostname(), ClientDestination: oldClient,
		ChordDestination: chordNode, TunnelDestination: tunnelNode,
	}
	as.True(serv.routeCache.SetWithTTL(link.GetHostname(), &routesResult{routes: []*protocol.TunnelRoute{route}}, 128, routePositiveTTL))
	fresh := &protocol.TunnelRoute{
		Hostname: link.GetHostname(), ClientDestination: newClient,
		ChordDestination: chordNode, TunnelDestination: tunnelNode,
	}
	wire, err := fresh.MarshalVT()
	as.NoError(err)
	lookupStarted := make(chan struct{}, 1)
	finishLookup := make(chan struct{})
	node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), 1))).Return(wire, nil).Once().Run(func(mock.Arguments) {
		lookupStarted <- struct{}{}
		<-finishLookup
	})
	for i := 2; i <= tun.NumRedundantLinks; i++ {
		node.On("Get", mock.Anything, []byte(tun.RoutingKey(link.GetHostname(), i))).Return(nil, nil).Once()
	}
	const callers = 12
	started := make(chan struct{}, callers)
	release := make(chan struct{})
	releaseDelayed := make(chan struct{})
	var attempts atomic.Int32
	clientT.On("Identity").Return(tunnelNode)
	clientT.On("DialStream", mock.Anything, oldClient, protocol.Stream_DIRECT).Return(nil, transport.ErrNoDirect).Times(callers).Run(func(mock.Arguments) {
		delayed := attempts.Add(1) == callers
		started <- struct{}{}
		if delayed {
			<-releaseDelayed
		} else {
			<-release
		}
	})
	clientT.On("DialStream", mock.Anything, newClient, protocol.Stream_DIRECT).Return(nil, transport.ErrNoDirect).Times(callers)
	results := make(chan error, callers)
	for range callers {
		go func() {
			_, err := serv.DialClient(t.Context(), link)
			results <- err
		}()
	}
	for range callers {
		awaitRouteResult(t, started)
	}
	close(release)
	awaitRouteResult(t, lookupStarted)
	close(finishLookup)
	for range callers - 1 {
		as.ErrorIs(awaitRouteResult(t, results), tun.ErrTunnelClientNotConnected)
	}
	// This caller exhausted the old generation after its replacement was cached.
	close(releaseDelayed)
	as.ErrorIs(awaitRouteResult(t, results), tun.ErrTunnelClientNotConnected)
	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestRouteLookupCancelledWaiterDoesNotCancelSharedLoad(t *testing.T) {
	as := require.New(t)
	_, node, clientT, _, serv := getFixture(t, as)
	t.Cleanup(serv.routeCache.Close)
	t.Cleanup(serv.keylessCache.Close)
	hostname := "cancelled.example.com"
	client, chordNode, tunnelNode := getIdentities()
	clientT.On("Identity").Return(tunnelNode)
	wire, err := (&protocol.TunnelRoute{
		Hostname: hostname, ClientDestination: client,
		ChordDestination: chordNode, TunnelDestination: tunnelNode,
	}).MarshalVT()
	as.NoError(err)
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	node.On("Get", mock.Anything, []byte(tun.RoutingKey(hostname, 1))).Return(wire, nil).Once().Run(func(args mock.Arguments) {
		started <- args.Get(0).(context.Context)
		<-release
	})
	for i := 2; i <= tun.NumRedundantLinks; i++ {
		node.On("Get", mock.Anything, []byte(tun.RoutingKey(hostname, i))).Return(nil, nil).Once()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := serv.lookupRoutes(ctx, hostname, nil)
		result <- err
	}()
	lookupCtx := awaitRouteResult(t, started)
	cancel()
	as.ErrorIs(awaitRouteResult(t, result), context.Canceled)
	as.NoError(lookupCtx.Err())
	close(release)
	ret, err := serv.lookupRoutes(t.Context(), hostname, nil)
	as.NoError(err)
	as.NoError(ret.err)
	as.Len(ret.routes, 1)
	node.AssertExpectations(t)
}

func awaitRouteResult[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case ret := <-ch:
		return ret
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for route operation")
		var zero T
		return zero
	}
}
