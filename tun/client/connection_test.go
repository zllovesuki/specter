package client

import (
	"context"
	"errors"
	"os"
	"testing"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap/zaptest"
)

func TestPublishPreferenceRTT(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx := t.Context()

	token := &protocol.ClientToken{
		Token: []byte("test"),
	}
	cl := &protocol.Node{
		Id: chord.Random(),
	}

	der, cert, key := makeCertificate(as, logger, cl, token, nil)
	cfg := &Config{
		path:        file.Name(),
		router:      skipmap.NewString[route](),
		Apex:        testApex,
		Certificate: cert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:2345",
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, _, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, true, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)
}

func TestReconciliationRecoversFromCompleteOutage(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{{
		Hostname: "configured",
		Target:   "http://localhost:8080",
	}})
	original := c.getConnectedNodes()
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: original}, nil).Once()
	require.Empty(t, c.SyncConfigTunnels(t.Context()).Error)
	saved, err := os.ReadFile(c.Configuration.path)
	require.NoError(t, err)

	gateway := mock.MatchedBy(func(ctx context.Context) bool {
		return rpc.GetNode(ctx).GetAddress() == original[0].GetAddress()
	})
	apex := mock.MatchedBy(func(ctx context.Context) bool {
		return rpc.GetNode(ctx).GetAddress() == c.Configuration.Apex
	})
	tunnelClient.TunnelService.On("Ping", gateway, mock.Anything).
		Return(nil, errors.New("gateway disconnected")).Once()
	tunnelClient.TunnelService.On("Ping", apex, mock.Anything).
		Return(nil, errors.New("apex temporarily unavailable")).Once()

	// Losing every gateway clears prior publication acknowledgements even when
	// the first bootstrap fails. Recovery remains pending for a later pass.
	c.reconcileConnections(t.Context())
	require.Empty(t, c.getConnectedNodes())
	require.True(t, c.getStatus().Pending)
	require.False(t, c.getStatus().Synchronization.Tunnels[0].Published)
	tunnelClient.TunnelService.AssertNumberOfCalls(t, "PublishTunnel", 1)

	peer := &protocol.Node{
		Id:      2,
		Address: "peer.example.com",
	}
	recovered := []*protocol.Node{original[0], peer}
	tunnelClient.TunnelService.On("Ping", apex, mock.Anything).
		Return(&protocol.ClientPingResponse{Node: original[0]}, nil).Once()
	tunnelClient.TunnelService.On("Ping", gateway, mock.Anything).
		Return(&protocol.ClientPingResponse{Node: original[0]}, nil).Once()
	tunnelClient.TunnelService.On("Ping", mock.MatchedBy(func(ctx context.Context) bool {
		return rpc.GetNode(ctx).GetAddress() == peer.GetAddress()
	}), mock.Anything).Return(&protocol.ClientPingResponse{Node: peer}, nil).Twice()
	tunnelClient.TunnelService.On("GetNodes", gateway, mock.Anything).
		Return(&protocol.GetNodesResponse{Nodes: recovered}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "configured" && endpointSignature(req.GetServers()) == endpointSignature(recovered)
	})).Return(&protocol.PublishTunnelResponse{Published: recovered}, nil).Once()

	// One pass bootstraps, discovers another peer, and republishes without a
	// separate worker or a delay between those steps.
	c.reconcileConnections(t.Context())
	require.Equal(t, endpointSignature(recovered), endpointSignature(c.getConnectedNodes()))
	status := c.getStatus()
	require.False(t, status.Pending)
	require.Empty(t, status.Synchronization.Error)
	require.Equal(t, 2, status.Synchronization.Tunnels[0].PublishedEndpoints)
	onDisk, err := os.ReadFile(c.Configuration.path)
	require.NoError(t, err)
	require.Equal(t, saved, onDisk, "reconnection must not rewrite the configuration")
}
