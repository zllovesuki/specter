package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

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

	ret, err := serv.routeCacheLoader(context.Background(), hostname)

	as.NoError(err)
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

			ret, err := serv.routeCacheLoader(context.Background(), hostname)

			as.NoError(err)
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

	ret, err := serv.routeCacheLoader(context.Background(), hostname)

	as.NoError(err)
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
