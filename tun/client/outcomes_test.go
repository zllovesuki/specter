package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap/zaptest"
)

func newOutcomeTestClient(t *testing.T, tunnels []Tunnel) (*Client, *mockTunnelClient) {
	t.Helper()

	cfg := &Config{
		path:    filepath.Join(t.TempDir(), "client.yml"),
		router:  skipmap.NewString[route](),
		Version: 2,
		Apex:    testApex,
		Tunnels: tunnels,
	}
	require.NoError(t, cfg.validate())
	cfg.buildRouter()
	require.NoError(t, cfg.writeFile())

	tunnelClient := new(mockTunnelClient)
	c := &Client{
		ClientConfig: ClientConfig{
			Logger:        zaptest.NewLogger(t),
			Configuration: cfg,
		},
		tunnelClient: tunnelClient,
		forwarder: &forwarder{
			logger:     zaptest.NewLogger(t),
			rootDomain: atomic.NewString(testApex),
			proxies:    skipmap.NewString[*httpProxy](),
		},
		connections: skipmap.NewString[*protocol.Node](),
	}
	c.connections.Store("gateway.example.com", &protocol.Node{
		Id:      1,
		Address: "gateway.example.com",
	})
	t.Cleanup(func() { tunnelClient.TunnelService.AssertExpectations(t) })
	return c, tunnelClient
}

func TestLocalReloadRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		remove  bool
	}{
		{name: "invalid YAML", content: "tunnels: ["},
		{name: "invalid target", content: "version: 2\ntunnels:\n  - hostname: changed\n    target: unsupported://localhost\n"},
		{name: "unreadable file", remove: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := Tunnel{Hostname: "existing", Target: "http://localhost:8080"}
			c, tunnelClient := newOutcomeTestClient(t, []Tunnel{original})
			if tc.remove {
				require.NoError(t, os.Remove(c.Configuration.path))
			} else {
				require.NoError(t, os.WriteFile(c.Configuration.path, []byte(tc.content), 0600))
			}

			response := httptest.NewRecorder()
			c.localHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/reload", nil))

			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Header().Get("Content-Type"), "application/json")
			var result SyncResult
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.NotEmpty(t, result.Error)
			require.False(t, result.Applied)
			require.False(t, result.Saved)
			require.Len(t, c.Configuration.Tunnels, 1)
			require.Equal(t, original.Hostname, c.Configuration.Tunnels[0].Hostname)
			require.Equal(t, original.Target, c.Configuration.Tunnels[0].Target)
			route, found := c.Configuration.router.Load(original.Hostname)
			require.True(t, found)
			require.Equal(t, original.Target, route.parsed.String())
			tunnelClient.TunnelService.AssertNotCalled(t, "PublishTunnel", mock.Anything, mock.Anything)
		})
	}
}

func TestLocalReloadReportsPartialPublishFailure(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{
		{Hostname: "published", Target: "http://localhost:8080"},
		{Hostname: "failed", Target: "tcp://localhost:5432"},
	})

	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil)
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "published"
	})).Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		return req.GetHostname() == "failed"
	})).Return(nil, errors.New("publication rejected")).Once()

	response := httptest.NewRecorder()
	c.localHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/reload", nil))

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Contains(t, response.Header().Get("Content-Type"), "application/json")
	var result SyncResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Applied)
	require.True(t, result.Saved)
	require.Contains(t, result.Error, "publication rejected")
	require.Len(t, result.Tunnels, 2)

	byHostname := make(map[string]TunnelSyncResult, len(result.Tunnels))
	for _, tunnel := range result.Tunnels {
		byHostname[tunnel.Hostname] = tunnel
	}
	published := byHostname["published"]
	require.Equal(t, "http://localhost:8080", published.Target)
	require.True(t, published.Published)
	require.Equal(t, 1, published.PublishedEndpoints)
	require.Empty(t, published.Error)
	failed := byHostname["failed"]
	require.Equal(t, "tcp://localhost:5432", failed.Target)
	require.False(t, failed.Published)
	require.Zero(t, failed.PublishedEndpoints)
	require.Contains(t, failed.Error, "publication rejected")

	onDisk, err := NewConfig(c.Configuration.path)
	require.NoError(t, err)
	require.Len(t, onDisk.Tunnels, 2)
}

func TestLocalTunnelRemovalReportsPersistenceFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		result any
	}{
		{name: "unpublish", method: "UnpublishTunnel", result: &protocol.UnpublishTunnelResponse{}},
		{name: "release", method: "ReleaseTunnel", result: &protocol.ReleaseTunnelResponse{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, tunnelClient := newOutcomeTestClient(t, []Tunnel{
				{Hostname: "existing", Target: "http://localhost:8080"},
			})
			tunnelClient.TunnelService.On(tc.method, mock.Anything, mock.MatchedBy(func(req any) bool {
				if req, ok := req.(*protocol.UnpublishTunnelRequest); ok {
					return req.GetHostname() == "existing"
				}
				if req, ok := req.(*protocol.ReleaseTunnelRequest); ok {
					return req.GetHostname() == "existing"
				}
				return false
			})).Return(tc.result, nil).Once()

			// A regular file cannot contain the config path, even when tests run as root.
			originalPath := c.Configuration.path
			originalContents, err := os.ReadFile(originalPath)
			require.NoError(t, err)
			c.Configuration.path = filepath.Join(originalPath, "client.yml")

			response := httptest.NewRecorder()
			c.localHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/"+tc.name+"/existing", nil))

			require.Equal(t, http.StatusInternalServerError, response.Code)
			require.Contains(t, response.Header().Get("Content-Type"), "application/json")
			var result SyncResult
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.NotEmpty(t, result.Error)
			require.True(t, result.Applied, "successful network removal must be distinguished from persistence failure")
			require.False(t, result.Saved)
			require.Empty(t, c.GetCurrentConfig().Tunnels)
			_, found := c.Configuration.router.Load("existing")
			require.False(t, found, "a successful network removal must also remove the live route")
			contents, err := os.ReadFile(originalPath)
			require.NoError(t, err)
			require.Equal(t, originalContents, contents)

			require.False(t, c.getStatus().Pending, "a save failure must not schedule publication")
			require.Nil(t, c.getStatus().RetryAt)

			// An explicit reload accepts the file's contents, including the old entry.
			c.Configuration.path = originalPath
			tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
				Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
			tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
				Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()
			reloadResponse := httptest.NewRecorder()
			c.localHandler().ServeHTTP(reloadResponse, httptest.NewRequest(http.MethodPost, "/api/reload", nil))
			require.Equal(t, http.StatusNoContent, reloadResponse.Code)
			require.Equal(t, "existing", c.GetCurrentConfig().Tunnels[0].Hostname)
			require.True(t, c.getStatus().Synchronization.Saved)
			_, found = c.Configuration.router.Load("existing")
			require.True(t, found)
		})
	}
}

func TestLocalStatusRemainsAvailableDuringPublication(t *testing.T) {
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{
		{Hostname: "existing", Target: "http://localhost:8080"},
	})
	c.Configuration.Certificate = "private-certificate-test-value"
	c.Configuration.PrivKey = "private-key-test-value"
	c.syncStateMu.Lock()
	c.lastSync = SyncResult{Tunnels: []TunnelSyncResult{
		{Hostname: "removed", Target: "http://localhost:9090", Published: true, PublishedEndpoints: 1},
		{Hostname: "existing", Target: "http://localhost:7070", Published: true, PublishedEndpoints: 1},
	}}
	c.syncStateMu.Unlock()

	publicationStarted := make(chan struct{})
	publicationRelease := make(chan struct{})
	publicationDone := make(chan SyncResult, 1)
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) {
			close(publicationStarted)
			<-publicationRelease
		}).Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()
	go func() { publicationDone <- c.SyncConfigTunnels(t.Context()) }()
	defer func() {
		close(publicationRelease)
		result := <-publicationDone
		require.Empty(t, result.Error)
	}()
	select {
	case <-publicationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("publication did not start")
	}

	statusDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		c.localHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		statusDone <- response
	}()
	var response *httptest.ResponseRecorder
	select {
	case response = <-statusDone:
	case <-time.After(2 * time.Second):
		t.Fatal("status endpoint blocked on publication")
	}

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Contains(t, response.Header().Get("Content-Type"), "application/json")
	var status ClientStatus
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Equal(t, testApex, status.Apex)
	require.Len(t, status.ConnectedNodes, 1)
	require.Equal(t, "gateway.example.com", status.ConnectedNodes[0].GetAddress())
	require.Equal(t, []TunnelSyncResult{
		{Hostname: "existing", Target: "http://localhost:8080"},
	}, status.Synchronization.Tunnels, "status must reflect configured targets, not stale acknowledgements")
	require.NotContains(t, response.Body.String(), "private-certificate-test-value")
	require.NotContains(t, response.Body.String(), "private-key-test-value")
	require.NotContains(t, response.Body.String(), `"certificate"`)
	require.NotContains(t, response.Body.String(), `"privKey"`)
}

func TestLocalUnpublishClearsSynchronization(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "published"},
		{name: "publication failed", err: errors.New("publication rejected")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, tunnelClient := newOutcomeTestClient(t, []Tunnel{{Hostname: "existing", Target: "http://localhost:8080"}})
			tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
				Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
			tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
				Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, tc.err).Once()
			tunnelClient.TunnelService.On("UnpublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.UnpublishTunnelRequest) bool {
				return req.GetHostname() == "existing"
			})).Return(&protocol.UnpublishTunnelResponse{}, nil).Once()
			initial := c.SyncConfigTunnels(t.Context())
			require.Equal(t, tc.err != nil, c.getStatus().Pending)
			require.Equal(t, tc.err == nil, initial.Tunnels[0].Published)

			handler := c.localHandler()
			removalResponse := httptest.NewRecorder()
			handler.ServeHTTP(removalResponse, httptest.NewRequest(http.MethodPost, "/api/unpublish/existing", nil))
			require.Equal(t, http.StatusOK, removalResponse.Code)
			statusResponse := httptest.NewRecorder()
			handler.ServeHTTP(statusResponse, httptest.NewRequest(http.MethodGet, "/api/status", nil))
			require.Equal(t, http.StatusOK, statusResponse.Code)
			var status ClientStatus
			require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &status))
			require.Equal(t, []TunnelSyncResult{}, status.Synchronization.Tunnels, "empty configuration must serialize as an empty array")
			require.True(t, status.Synchronization.Applied)
			require.True(t, status.Synchronization.Saved)
			require.False(t, status.Pending)
			require.Nil(t, status.RetryAt)
			require.Empty(t, status.Synchronization.Error)
		})
	}
}

func TestGatewayChangePreservesUnreloadedConfigurationEdits(t *testing.T) {
	const originalTarget = "http://localhost:8080"
	const editedTarget = "http://localhost:9090"
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{
		{Hostname: "existing", Target: originalTarget},
	})
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{}, nil).Once()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Once()

	initial := c.SyncConfigTunnels(t.Context())
	require.Empty(t, initial.Error)
	require.True(t, initial.Saved)

	// Discovering another gateway must not rewrite edits awaiting explicit reload.
	nodes := append(c.getConnectedNodes(), &protocol.Node{Id: 2, Address: "second.example.com"})
	tunnelClient.TunnelService.On("GetNodes", mock.Anything, mock.Anything).
		Return(&protocol.GetNodesResponse{Nodes: nodes}, nil).Once()
	for _, node := range nodes {
		tunnelClient.TunnelService.On("Ping", mock.MatchedBy(func(ctx context.Context) bool {
			return rpc.GetNode(ctx).GetId() == node.GetId()
		}), mock.Anything).Return(&protocol.ClientPingResponse{Node: node}, nil)
	}
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: nodes}, nil).Once()
	edited := c.GetCurrentConfig()
	edited.Tunnels[0].Target = editedTarget
	require.NoError(t, edited.writeFile())
	c.reconcileConnections(t.Context())

	status := c.getStatus()
	require.False(t, status.Pending)
	require.Empty(t, status.Synchronization.Error)
	require.True(t, status.Synchronization.Tunnels[0].Published)
	require.Equal(t, 2, status.Synchronization.Tunnels[0].PublishedEndpoints)
	require.Equal(t, originalTarget, status.Synchronization.Tunnels[0].Target)
	require.Equal(t, originalTarget, c.GetCurrentConfig().Tunnels[0].Target)
	route, found := c.Configuration.router.Load("existing")
	require.True(t, found)
	require.Equal(t, originalTarget, route.parsed.String())
	onDisk, err := NewConfig(edited.path)
	require.NoError(t, err)
	require.Equal(t, editedTarget, onDisk.Tunnels[0].Target)
}
