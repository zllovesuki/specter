package client

import (
	"errors"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPublicationRetryInvalidatesAcknowledgementAfterGatewayReordering(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{
		{Hostname: "first", Target: "http://localhost:8080"},
		{Hostname: "pending", Target: "http://localhost:9090"},
	})
	a := c.getConnectedNodes()[0]
	b := &protocol.Node{Id: 2, Address: "second-gateway.example.com"}
	c.connections.Store(b.GetAddress(), b)
	ab := []*protocol.Node{a, b}
	ba := []*protocol.Node{b, a}

	aRTT := &rtt.Statistics{Average: time.Millisecond}
	bRTT := &rtt.Statistics{Average: 2 * time.Millisecond}
	recorder := new(mocks.Measurement)
	recorder.On("Snapshot", rtt.MakeMeasurementKey(a), mock.Anything).Return(aRTT)
	recorder.On("Snapshot", rtt.MakeMeasurementKey(b), mock.Anything).Return(bRTT)
	c.Recorder = recorder
	t.Cleanup(func() { recorder.AssertExpectations(t) })

	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	firstAt := func(nodes []*protocol.Node) any {
		return mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
			return req.GetHostname() == "first" && len(req.GetServers()) == 2 &&
				req.GetServers()[0].GetId() == nodes[0].GetId() &&
				req.GetServers()[1].GetId() == nodes[1].GetId()
		})
	}
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, firstAt(ab)).
		Return(&protocol.PublishTunnelResponse{Published: ab}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, firstAt(ba)).
		Return(&protocol.PublishTunnelResponse{Published: []*protocol.Node{b}}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, firstAt(ab)).
		Return(&protocol.PublishTunnelResponse{Published: ab}, nil).Once()
	pending := mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "pending"
	})
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, pending).
		Return(nil, errors.New("temporary publication failure")).Twice()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, pending).
		Return(&protocol.PublishTunnelResponse{Published: ab}, nil).Once()

	initial := c.SyncConfigTunnels(t.Context())
	require.True(t, initial.Tunnels[0].Published)
	require.Empty(t, initial.Tunnels[0].Error)
	require.NotEmpty(t, initial.Tunnels[1].Error)
	require.True(t, c.getStatus().Pending)

	// A partial attempt in the opposite order can overwrite numbered routing
	// slots, so the original successful acknowledgement is no longer valid.
	aRTT.Average, bRTT.Average = 2*time.Millisecond, time.Millisecond
	c.syncConfigTunnels(t.Context(), false)
	partial := c.getStatus()
	require.True(t, partial.Pending)
	require.Contains(t, partial.Synchronization.Tunnels[0].Error, "published 1 of 2")
	require.Equal(t, 1, partial.Synchronization.Tunnels[0].PublishedEndpoints)

	// Returning to the original order must repair those slots with a fresh RPC.
	aRTT.Average, bRTT.Average = time.Millisecond, 2*time.Millisecond
	c.syncConfigTunnels(t.Context(), false)
	status := c.getStatus()
	require.False(t, status.Pending)
	require.Nil(t, status.RetryAt)
	require.Empty(t, status.Synchronization.Error)
	for _, tunnel := range status.Synchronization.Tunnels {
		require.True(t, tunnel.Published)
		require.Equal(t, 2, tunnel.PublishedEndpoints)
		require.Empty(t, tunnel.Error)
	}
}
