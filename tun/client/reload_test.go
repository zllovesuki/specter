package client

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

func TestReloadDuringIncomingHTTPDelegation(t *testing.T) {
	oldTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "old:"+r.Host)
	}))
	defer oldTarget.Close()
	newTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "new:"+r.Host)
	}))
	defer newTarget.Close()
	c, tunnelClient := newOutcomeTestClient(t, []Tunnel{{
		Hostname: testHostname, Target: oldTarget.URL,
		ProxyHeaderMode: "custom", ProxyHeaderHost: "old.example.com",
	}})
	tunnelClient.TunnelService.On("RegisteredHostnames", mock.Anything, mock.Anything).
		Return(&protocol.RegisteredHostnamesResponse{Hostnames: []string{testHostname}}, nil).Twice()
	tunnelClient.TunnelService.On("PublishTunnel", mock.Anything, mock.Anything).
		Return(&protocol.PublishTunnelResponse{Published: c.getConnectedNodes()}, nil).Twice()

	captured, resume := make(chan struct{}), make(chan struct{}, 1)
	defer close(resume)
	var pause atomic.Bool
	c.forwarder.logger = zaptest.NewLogger(t, zaptest.WrapOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message == "Incoming connection from gateway" && pause.CompareAndSwap(true, false) {
			close(captured)
			<-resume
		}
		return nil
	})))

	link := &protocol.Link{Hostname: testHostname, Alpn: protocol.Link_HTTP}
	server := &forwardedTunnelServer{client: c}
	visitor := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return server.DialClient(ctx, link)
		},
	}}
	request := func(want string) {
		t.Helper()
		response, err := visitor.Get("http://" + testHostname + "/")
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, want, string(body))
	}
	request("old:old.example.com")

	// Stop an incoming delegation after it has selected the old route, then
	// reload before it can hand its connection to the proxy.
	pause.Store(true)
	delegation, remote := net.Pipe()
	defer delegation.Close()
	defer remote.Close()
	done := make(chan error, 1)
	go func() { done <- c.handleIncomingDelegation(t.Context(), link, delegation) }()
	<-captured
	next := c.GetCurrentConfig()
	next.Tunnels[0].Target = newTarget.URL
	next.Tunnels[0].ProxyHeaderHost = "new.example.com"
	require.NoError(t, next.writeFile())
	result := c.doReload(t.Context())
	require.Empty(t, result.Error)
	require.True(t, result.Applied)
	resume <- struct{}{}
	require.NoError(t, <-done)

	// The delayed delegation must not repopulate the cache with the old route,
	// including when a later reload has no configuration changes to invalidate.
	request("new:new.example.com")
	result = c.doReload(t.Context())
	require.Empty(t, result.Error)
	request("new:new.example.com")
}

func TestReloadOnSignal(t *testing.T) {
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
		Apex:        testApex,
		Certificate: cert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:1234",
			},
		},
	}
	as.NoError(cfg.validate())

	reload := make(chan os.Signal, 1)

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, _, assertion := setupClient(t, as, ctx, logger, nil, cfg, reload, m, false, 2)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	// send reload signal
	reload <- syscall.SIGHUP

	time.Sleep(time.Second)
}
