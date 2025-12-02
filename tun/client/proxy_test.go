package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"testing"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/util/pipe"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap/zaptest"
)

func TestJustHTTPProxy(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	}))
	defer ts.Close()

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
				Target: ts.URL,
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, t2, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, false, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	httpCl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: -1,
			DisableKeepAlives:   true,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				c1, err := t2.DialStream(ctx, cl, protocol.Stream_DIRECT)
				as.NoError(err)
				as.NoError(rpc.Send(c1, &protocol.Link{
					Alpn:     protocol.Link_HTTP,
					Hostname: testHostname,
				}))
				return c1, nil
			},
		},
	}
	resp, err := httpCl.Get("http://test/")
	as.NoError(err)
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	as.NoError(err)
	as.Equal("ok", string(buf))
}

func TestPipeHTTP(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		target string
		path   string
	)
	if runtime.GOOS == "windows" {
		path = "\\\\.\\pipe\\specterhttp"
		target = path
	} else {
		path = "/tmp/specterhttp.sock"
		target = "unix://" + path
	}

	pipeListener, err := pipe.ListenPipe(path)
	as.NoError(err)
	defer pipeListener.Close()

	svc := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "ok")
		}),
	}
	go svc.Serve(pipeListener)
	defer svc.Shutdown(ctx)

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
				Target: target,
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, t2, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, false, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	httpCl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: -1,
			DisableKeepAlives:   true,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				c1, err := t2.DialStream(ctx, cl, protocol.Stream_DIRECT)
				as.NoError(err)
				as.NoError(rpc.Send(c1, &protocol.Link{
					Alpn:     protocol.Link_HTTP,
					Hostname: testHostname,
				}))
				return c1, nil
			},
		},
	}
	resp, err := httpCl.Get("http://test/")
	as.NoError(err)
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	as.NoError(err)
	as.Equal("ok", string(buf))
}

func TestPipeTCP(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		target string
		path   string
	)
	if runtime.GOOS == "windows" {
		path = "\\\\.\\pipe\\spectertcp"
		target = path
	} else {
		path = "/tmp/spectertcp.sock"
		target = "unix://" + path
	}

	pipeListener, err := pipe.ListenPipe(path)
	as.NoError(err)
	defer pipeListener.Close()

	go func() {
		conn, err := pipeListener.Accept()
		as.NoError(err)
		conn.Write([]byte("hi"))
	}()

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
				Target: target,
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, t2, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, false, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	conn, err := t2.DialStream(ctx, cl, protocol.Stream_DIRECT)
	as.NoError(err)
	as.NoError(rpc.Send(conn, &protocol.Link{
		Alpn:     protocol.Link_TCP,
		Hostname: testHostname,
	}))

	status := &protocol.TunnelStatus{}
	rpc.BoundedReceive(conn, status, 1024)
	as.Equal(protocol.TunnelStatusCode_STATUS_OK, status.GetStatus())

	buf := make([]byte, 2)
	n, err := io.ReadFull(conn, buf)
	as.NoError(err)
	as.Equal(2, n)
	as.Equal("hi", string(buf))
}

func TestHTTPProxyHostHeaderDefault(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", r.Host)
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	as.NoError(err)

	c := &Client{
		ClientConfig: ClientConfig{
			Logger: logger,
		},
		rootDomain: atomic.NewString(testApex),
		proxies:    skipmap.NewString[*httpProxy](),
	}

	r := route{
		parsed: u,
	}

	hp := c.getHTTPProxy(context.Background(), testHostname, r)

	req := httptest.NewRequest("GET", "http://example/", nil)
	rr := httptest.NewRecorder()
	hp.forwarder.Handler.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	as.NoError(err)
	as.Equal(u.Host, string(body))
}

func TestHTTPProxyHostHeaderHostnameMode(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", r.Host)
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	as.NoError(err)

	c := &Client{
		ClientConfig: ClientConfig{
			Logger: logger,
		},
		rootDomain: atomic.NewString(testApex),
		proxies:    skipmap.NewString[*httpProxy](),
	}

	r := route{
		parsed:          u,
		proxyHeaderMode: "hostname",
	}

	hp := c.getHTTPProxy(context.Background(), testHostname, r)

	req := httptest.NewRequest("GET", "http://example/", nil)
	rr := httptest.NewRecorder()
	hp.forwarder.Handler.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	as.NoError(err)
	as.Equal(fmt.Sprintf("%s.%s", testHostname, testApex), string(body))
}

func TestHTTPProxyHostHeaderCustomMode(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", r.Host)
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	as.NoError(err)

	c := &Client{
		ClientConfig: ClientConfig{
			Logger: logger,
		},
		rootDomain: atomic.NewString(testApex),
		proxies:    skipmap.NewString[*httpProxy](),
	}

	customHost := "custom.example.com"
	r := route{
		parsed:          u,
		proxyHeaderMode: "custom",
		proxyHeaderHost: customHost,
	}

	hp := c.getHTTPProxy(context.Background(), testHostname, r)

	req := httptest.NewRequest("GET", "http://example/", nil)
	rr := httptest.NewRecorder()
	hp.forwarder.Handler.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	as.NoError(err)
	as.Equal(customHost, string(body))
}
