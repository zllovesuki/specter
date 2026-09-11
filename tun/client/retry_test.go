package client

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func makeSyncRetryDue(c *Client) {
	c.syncStateMu.Lock()
	c.nextSync = time.Now().Add(-time.Second)
	c.syncStateMu.Unlock()
}

func expectStableGateway(c *Client, tunnelClient *mockTunnelClient, maintenanceError error) {
	nodes := c.getConnectedNodes()
	tunnelClient.TunnelService.On("Ping", mock.Anything, mock.Anything).
		Return(&protocol.ClientPingResponse{Node: nodes[0]}, nil)
	tunnelClient.TunnelService.On("GetNodes", mock.Anything, mock.Anything).
		Return(&protocol.GetNodesResponse{Nodes: nodes}, maintenanceError)
}

func TestPublicationRetriesWithStableConnections(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{{Target: "http://localhost:8080"}})
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(nil, errors.New("temporary hostname lookup failure")).Once()
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("GenerateHostname", mock.Anything, mock.Anything).
		Return(&protocol.GenerateHostnameResponse{Hostname: "generated"}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()
	// Candidate discovery can fail independently of working connected gateways.
	expectStableGateway(c, tunnelClient, errors.New("temporary candidate lookup failure"))

	initial := c.SyncConfigTunnels(t.Context())
	require.Contains(t, initial.Error, "temporary hostname lookup failure")
	require.True(t, c.getStatus().Pending)
	require.False(t, initial.Tunnels[0].Published)

	// A normal maintenance cycle respects backoff, even with stable gateways.
	c.reconcileConnections(t.Context())
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "RegisteredHostnames", 1)
	makeSyncRetryDue(c)
	c.reconcileConnections(t.Context())

	status := c.getStatus()
	require.False(t, status.Pending)
	require.Nil(t, status.RetryAt)
	require.Empty(t, status.Synchronization.Error)
	require.True(t, status.Synchronization.Saved)
	require.True(t, status.Synchronization.Tunnels[0].Published)
	require.Equal(t, "generated", status.Synchronization.Tunnels[0].Hostname)

	// Converged tunnels do not get republished every maintenance interval.
	c.reconcileConnections(t.Context())
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "PublishTunnel", 1)
}

func TestPublicationRetryKeepsAssignedNamesAndSuccessfulTunnels(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{
		{Hostname: "already-published", Target: "http://localhost:8080"},
		{Target: "tcp://localhost:5432"},
	})
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("GenerateHostname", mock.Anything, mock.Anything).
		Return(&protocol.GenerateHostnameResponse{Hostname: "generated"}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "already-published"
	})).Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "generated"
	})).Return(nil, errors.New("temporary publication failure")).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "generated"
	})).Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()
	originalPath := c.Configuration.path
	c.Configuration.path = filepath.Join(originalPath, "client.yml")
	initial := c.SyncConfigTunnels(t.Context())
	require.False(t, initial.Saved)
	require.Contains(t, initial.Error, "temporary publication failure")
	require.Equal(t, "generated", c.GetCurrentConfig().Tunnels[1].Hostname)

	// Publication may recover without retrying the failed save.
	c.Configuration.path = originalPath
	c.syncConfigTunnels(t.Context(), false)
	status := c.getStatus()
	require.False(t, status.Pending)
	require.Nil(t, status.RetryAt)
	require.False(t, status.Synchronization.Saved)
	require.Contains(t, status.Synchronization.Error, "not saved")
	for _, tunnel := range status.Synchronization.Tunnels {
		require.True(t, tunnel.Published)
	}
	onDisk, err := NewConfig(originalPath)
	require.NoError(t, err)
	require.Empty(t, onDisk.Tunnels[1].Hostname, "publication retry must leave the file untouched")
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "GenerateHostname", 1)
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "PublishTunnel", 3)
}

func TestAmbiguousHostnameGenerationReusesRegisteredName(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{{Target: "http://localhost:8080"}})
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{Hostnames: []string{"created-before-response-failed"}}, nil).Once()
	tunnelClient.TunnelService.On("GenerateHostname", mock.Anything, mock.Anything).
		Return(nil, errors.New("response lost after hostname creation")).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "created-before-response-failed"
	})).Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()
	require.NotEmpty(t, c.SyncConfigTunnels(t.Context()).Error)
	c.syncConfigTunnels(t.Context(), false)
	require.False(t, c.getStatus().Pending)
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "GenerateHostname", 1)
}

func TestPublicationRetryStopsOnClientClose(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{{Hostname: "configured", Target: "http://localhost:8080"}})
	c.closeCh = make(chan struct{})
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(nil, errors.New("initial publication failure")).Once()
	entered := make(chan struct{})
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			close(entered)
			<-args.Get(0).(context.Context).Done()
		}).Return(nil, context.Canceled).Once()
	expectStableGateway(c, tunnelClient, nil)
	require.NotEmpty(t, c.SyncConfigTunnels(t.Context()).Error)
	makeSyncRetryDue(c)
	c.closeWg.Add(1)
	finished := make(chan struct{})
	go func() {
		c.periodicReconnection(t.Context())
		close(finished)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("background retry never reached publication")
	}
	close(c.closeCh)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("background retry did not stop after client close")
	}
}

func TestPublicationRetryRepairsPartialGatewayAcknowledgement(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{{Hostname: "configured", Target: "http://localhost:8080"}})
	c.connections.Store("second.example.com", &protocol.Node{Id: 2, Address: "second.example.com"})
	nodes := c.getConnectedNodes()
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: nodes[:1]}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: nodes}, nil).Once()
	initial := c.SyncConfigTunnels(t.Context())
	require.Contains(t, initial.Error, "published 1 of 2 connected gateways")
	require.True(t, initial.Tunnels[0].Published, "a usable publication remains distinguishable from a total failure")
	require.Equal(t, 1, initial.Tunnels[0].PublishedEndpoints)
	require.True(t, c.getStatus().Pending)
	c.syncConfigTunnels(t.Context(), false)

	status := c.getStatus()
	require.False(t, status.Pending)
	require.Empty(t, status.Synchronization.Error)
	require.Equal(t, 2, status.Synchronization.Tunnels[0].PublishedEndpoints)
}

func TestHostnameGenerationFailsOverAfterReconciliation(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{
		{Target: "http://localhost:8080"},
		{Target: "tcp://localhost:5432"},
	})
	c.connections.Store("second.example.com", &protocol.Node{Id: 2, Address: "second.example.com"})
	nodes := c.getConnectedNodes()
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Twice()
	tunnelClient.TunnelService.On("GenerateHostname", mock.MatchedBy(func(ctx context.Context) bool {
		return rpc.GetNode(ctx).GetAddress() == nodes[0].GetAddress()
	}), mock.Anything).Return(nil, errors.New("preferred gateway cannot create hostnames")).Once()
	for _, hostname := range []string{"first-generated", "second-generated"} {
		tunnelClient.TunnelService.On("GenerateHostname", mock.MatchedBy(func(ctx context.Context) bool {
			return rpc.GetNode(ctx).GetAddress() == nodes[1].GetAddress()
		}), mock.Anything).Return(&protocol.GenerateHostnameResponse{Hostname: hostname}, nil).Once()
	}
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: nodes}, nil).Twice()
	initial := c.SyncConfigTunnels(t.Context())
	require.Contains(t, initial.Error, "preferred gateway cannot create hostnames")
	// No other generation can run until registered names reconcile the ambiguous
	// response, even when another unnamed tunnel is waiting in this same pass.
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "GenerateHostname", 1)
	require.Empty(t, c.GetCurrentConfig().Tunnels[0].Hostname)
	require.Empty(t, c.GetCurrentConfig().Tunnels[1].Hostname)

	c.syncConfigTunnels(t.Context(), false)
	status := c.getStatus()
	require.False(t, status.Pending)
	require.Empty(t, status.Synchronization.Error)
	require.Equal(t, "first-generated", status.Synchronization.Tunnels[0].Hostname)
	require.Equal(t, "second-generated", status.Synchronization.Tunnels[1].Hostname)
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "RegisteredHostnames", 2)
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "GenerateHostname", 3)
}
