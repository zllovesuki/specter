package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRouteCacheLoaderAllNotFound(t *testing.T) {
	as := require.New(t)

	_, node, _, _, serv := getFixture(t, as)

	hostname := "all-not-found.example.com"
	link := &protocol.Link{Hostname: hostname}
	expected := getExpected(link)

	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		return assertBytes(k, expected...)
	})).Return([]byte{}, nil)

	ret := serv.routeCacheLoader(context.Background(), hostname)

	as.ErrorIs(ret.Value.err, tun.ErrDestinationNotFound)
	as.Equal(routeNegativeTTL, ret.TTL)
	as.EqualValues(int64(8), ret.Cost)
	as.Nil(ret.Value.routes)

	node.AssertExpectations(t)
}

func TestRouteCacheLoaderLookupFailure(t *testing.T) {
	for numErrors := 1; numErrors <= tun.NumRedundantLinks; numErrors++ {
		t.Run(fmt.Sprintf("%d_errors", numErrors), func(t *testing.T) {
			as := require.New(t)
			_, node, _, _, serv := getFixture(t, as)
			hostname := "lookup-failure.example.com"

			for i := 1; i <= tun.NumRedundantLinks; i++ {
				var lookupErr error
				if i <= numErrors {
					lookupErr = errors.New("boom")
				}
				node.On("Get", mock.Anything, []byte(tun.RoutingKey(hostname, i))).
					Return(([]byte)(nil), lookupErr).Once()
			}

			ret := serv.routeCacheLoader(context.Background(), hostname)

			as.ErrorIs(ret.Value.err, tun.ErrLookupFailed)
			as.Equal(routeFailedTTL, ret.TTL)
			as.EqualValues(int64(16), ret.Cost)
			as.Nil(ret.Value.routes)
			node.AssertExpectations(t)
		})
	}
}

func TestRouteCacheLoaderSuccessPrioritizesDirect(t *testing.T) {
	as := require.New(t)

	_, node, clientT, _, serv := getFixture(t, as)
	cli, cht, tn := getIdentities()

	hostname := "success.example.com"

	indirectRoute := &protocol.TunnelRoute{
		ClientDestination: cli,
		ChordDestination:  cht,
		TunnelDestination: &protocol.Node{Address: "remote:123"},
		Hostname:          hostname,
	}
	indirectBuf, err := indirectRoute.MarshalVT()
	as.NoError(err)

	directRoute := &protocol.TunnelRoute{
		ClientDestination: cli,
		ChordDestination:  cht,
		TunnelDestination: tn,
		Hostname:          hostname,
	}
	directBuf, err := directRoute.MarshalVT()
	as.NoError(err)

	keys := [][]byte{
		[]byte(tun.RoutingKey(hostname, 1)),
		[]byte(tun.RoutingKey(hostname, 2)),
		[]byte(tun.RoutingKey(hostname, 3)),
	}

	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		return bytes.Equal(k, keys[0])
	})).Return(indirectBuf, nil)

	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		return bytes.Equal(k, keys[1])
	})).Return(directBuf, nil)

	node.On("Get", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		return bytes.Equal(k, keys[2])
	})).Return(([]byte)(nil), errors.New("temporary lookup failure"))

	clientT.On("Identity").Return(tn)

	ret := serv.routeCacheLoader(context.Background(), hostname)

	as.NoError(ret.Value.err)
	as.Len(ret.Value.routes, 2)
	as.Equal(routePositiveTTL, ret.TTL)
	as.True(ret.Cost > 0)

	// the directly connected route (matching TunnelTransport.Identity) should be first
	as.Equal(tn.GetAddress(), ret.Value.routes[0].GetTunnelDestination().GetAddress())
	as.Equal("remote:123", ret.Value.routes[1].GetTunnelDestination().GetAddress())

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestEphemeralRoutes(t *testing.T) {
	s, n, _, _ := sessionFixture(t)
	label, inv, err := tun.EphemeralLabel(s.Chord.ID(), []byte("key"))
	require.NoError(t, err)
	local := s.routeCacheLoader(t.Context(), label)
	require.NoError(t, local.Value.err)
	require.GreaterOrEqual(t, local.Cost, int64(256))
	n.AssertNotCalled(t, "Get", mock.Anything, mock.Anything)
	remote := uint64(10)
	n.On("FindSuccessor", remote).Return(getVNode(&protocol.Node{Id: remote + 1}), nil).Once()
	missing := s.ephemeralRouteLoader(t.Context(), label, remote, inv)
	require.ErrorIs(t, missing.Value.err, tun.ErrDestinationNotFound)
	dst := &protocol.TunnelDestination{
		Chord: &protocol.Node{
			Id:      remote,
			Address: "remote",
		},
		Tunnel: &protocol.Node{Address: "remote tunnel"},
	}
	data, _ := dst.MarshalVT()
	n.On("FindSuccessor", remote).Return(getVNode(dst.Chord), nil).Once()
	n.On("Get", mock.Anything, []byte(tun.DestinationByChordKey(dst.Chord))).Return(data, nil).Once()
	found := s.ephemeralRouteLoader(t.Context(), label, remote, inv)
	require.NoError(t, found.Value.err)
	require.Equal(t, dst.Tunnel, found.Value.routes[0].TunnelDestination)
	require.Equal(t, tun.SessionAlias(inv), found.Value.routes[0].ClientDestination.Address)
	require.GreaterOrEqual(t, found.Cost, int64(256))
	blocked, release := make(chan struct{}, 16), make(chan struct{})
	n.On("FindSuccessor", uint64(11)).Run(func(mock.Arguments) { blocked <- struct{}{}; <-release }).Return(nil, errors.New("blocked lookup")).Times(16)
	for i := range 16 {
		go s.ephemeralRouteLoader(t.Context(), fmt.Sprint(i), 11, inv)
	}
	for range 16 {
		<-blocked
	}
	exhausted := s.ephemeralRouteLoader(t.Context(), "17", 11, inv)
	require.ErrorIs(t, exhausted.Value.err, tun.ErrLookupFailed)
	require.Equal(t, routeFailedTTL, exhausted.TTL)
	// Expired waiters must not free admission for still-running Chord traversals.
	time.Sleep(lookupTimeout + 10*time.Millisecond)
	require.Len(t, s.ephemeralLoads, 16)
	close(release)
	require.Eventually(t, func() bool { return len(s.ephemeralLoads) == 0 }, time.Second, time.Millisecond)
	n.AssertExpectations(t)
}
