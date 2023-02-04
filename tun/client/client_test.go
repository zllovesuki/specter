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
	"syscall"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/rtt"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/acceptor"
	"kon.nect.sh/specter/util/pipe"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

const (
	testHostname = "abcd"
	testApex     = "specter.dev"
)

func TestMain(m *testing.M) {
	checkInterval = time.Millisecond * 200
	m.Run()
}

func setupRPC(ctx context.Context,
	logger *zap.Logger,
	s protocol.TunnelService,
	router *transport.StreamRouter,
	acc *acceptor.HTTP2Acceptor,
	token *protocol.ClientToken,
	verifiedClient *protocol.Node,
) {
	tunTwirp := protocol.NewTunnelServiceServer(s, twirp.WithServerHooks(&twirp.ServerHooks{
		RequestRouted: func(ctx context.Context) (context.Context, error) {
			ctx = rpc.WithClientToken(ctx, token)
			ctx = rpc.WithCientIdentity(ctx, verifiedClient)
			return ctx, nil
		},
		Error: func(ctx context.Context, err twirp.Error) context.Context {
			logger.Error("error handling request", zap.Error(err))
			return ctx
		},
	}))

	rpcHandler := chi.NewRouter()
	rpcHandler.Use(middleware.Recoverer)
	rpcHandler.Use(util.LimitBody(1 << 10)) // 1KB
	rpcHandler.Mount(tunTwirp.PathPrefix(), rpc.ExtractAuthorizationHeader(tunTwirp))

	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return rpc.WithDelegation(ctx, c.(*transport.StreamDelegate))
		},
		MaxHeaderBytes: 1 << 10, // 1KB
		ReadTimeout:    time.Second * 3,
		Handler:        rpcHandler,
		ErrorLog:       util.GetStdLogger(logger, "rpc_server"),
	}

	go srv.Serve(acc)

	router.HandleTunnel(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		acc.Handle(delegate)
	})
}

func setupFakeNodes(rr *mocks.Measurement, s *mocks.TunnelService) []*protocol.Node {
	fakeNodes := []*protocol.Node{
		{
			Id:      1,
			Address: "192.168.0.1",
		},
		{
			Id:      2,
			Address: "192.168.0.2",
		},
		{
			Id:      3,
			Address: "192.168.0.3",
		},
	}

	latencies := []*rtt.Statistics{
		{
			Average: time.Hour,
		},
		{
			Average: time.Second,
		},
		{
			Average: time.Minute,
		},
	}

	for i, n := range fakeNodes {
		n := n
		// inject fake nodes. Dial target is available in delegation.Identity because PipeTransport()
		rr.On("Snapshot", mock.MatchedBy(func(key string) bool {
			return rtt.MakeMeasurementKey(n) == key
		}), mock.Anything).Return(latencies[i])
		s.On("Ping", mock.MatchedBy(func(ctx context.Context) bool {
			delegation := rpc.GetDelegation(ctx)
			return delegation.Identity.String() == n.String()
		}), mock.Anything).Return(&protocol.ClientPingResponse{
			Node: fakeNodes[i],
			Apex: testApex,
		}, nil).Maybe()
	}

	s.On("Ping", mock.Anything, mock.Anything).Return(&protocol.ClientPingResponse{
		Node: fakeNodes[0],
		Apex: testApex,
	}, nil).Maybe()
	s.On("GetNodes", mock.Anything, mock.Anything).Return(&protocol.GetNodesResponse{
		Nodes: fakeNodes,
	}, nil)
	s.On("GenerateHostname", mock.Anything, mock.Anything).Return(&protocol.GenerateHostnameResponse{
		Hostname: testHostname,
	}, nil)

	return fakeNodes
}

func setupClient(t *testing.T, as *require.Assertions, ctx context.Context, logger *zap.Logger, cl *protocol.Node, token *protocol.ClientToken, cfg *Config, reload <-chan os.Signal, m func(s *mocks.TunnelService, t1 *mocks.MemoryTransport), preference bool, times int) (
	*Client,
	*mocks.MemoryTransport,
	func(),
) {
	t1, t2 := mocks.PipeTransport()
	rr := new(mocks.Measurement)

	s := new(mocks.TunnelService)
	router := transport.NewStreamRouter(logger, nil, t2)
	acc := acceptor.NewH2Acceptor(nil)

	fakeNodes := setupFakeNodes(rr, s)

	if preference {
		s.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
			// ensure that the node with the lowest latency is the first preference
			return len(req.GetServers()) > 0 && req.GetServers()[0].GetAddress() == fakeNodes[1].GetAddress()
		})).Return(&protocol.PublishTunnelResponse{
			Published: fakeNodes,
		}, nil).Times(times)
	} else {
		s.On("PublishTunnel", mock.Anything, mock.Anything).Return(&protocol.PublishTunnelResponse{
			Published: fakeNodes,
		}, nil).Times(times)
	}

	if m != nil {
		m(s, t1)
	}

	go router.Accept(ctx)

	setupRPC(ctx, logger, s, router, acc, token, cl)

	client, err := NewClient(rpc.DisablePooling(ctx), ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		ServerTransport: t1,
		Recorder:        rr,
		ReloadSignal:    reload,
	})
	as.NoError(err)

	as.NoError(client.Register(ctx))

	as.NoError(client.Initialize(ctx))

	return client, t2, func() {
		acc.Close()
		rr.AssertExpectations(t)
		s.AssertExpectations(t)
	}
}

func TestPublishPreferenceRTT(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token := &protocol.ClientToken{
		Token: []byte("test"),
	}
	cl := &protocol.Node{
		Id: chord.Random(),
	}

	cfg := &Config{
		path:     file.Name(),
		router:   skipmap.NewString[*url.URL](),
		Apex:     testApex,
		ClientID: cl.GetId(),
		Token:    string(token.GetToken()),
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:2345",
			},
		},
	}
	as.NoError(cfg.validate())

	client, _, assertion := setupClient(t, as, ctx, logger, cl, token, cfg, nil, nil, true, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)
}

func TestReloadOnSignal(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token := &protocol.ClientToken{
		Token: []byte("test"),
	}
	cl := &protocol.Node{
		Id: chord.Random(),
	}

	cfg := &Config{
		path:     file.Name(),
		router:   skipmap.NewString[*url.URL](),
		Apex:     testApex,
		ClientID: cl.GetId(),
		Token:    string(token.GetToken()),
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:1234",
			},
		},
	}
	as.NoError(cfg.validate())

	reload := make(chan os.Signal, 1)

	client, _, assertion := setupClient(t, as, ctx, logger, cl, token, cfg, reload, nil, false, 2)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	// send reload signal
	reload <- syscall.SIGHUP

	time.Sleep(time.Second)
}

func TestRegisterAndProxy(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	as.NoError(err)
	defer tcpListener.Close()

	go func() {
		conn, err := tcpListener.Accept()
		as.NoError(err)
		conn.Write([]byte("hi"))
	}()

	token := &protocol.ClientToken{
		Token: []byte("test"),
	}
	cl := &protocol.Node{
		Id: chord.Random(),
	}

	// no token
	cfg := &Config{
		path:     file.Name(),
		router:   skipmap.NewString[*url.URL](),
		Apex:     testApex,
		ClientID: cl.GetId(),
		Tunnels: []Tunnel{
			{
				Target: fmt.Sprintf("tcp://%s", tcpListener.Addr()),
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport) {
		s.On("RegisterIdentity", mock.Anything, mock.MatchedBy(func(req *protocol.RegisterIdentityRequest) bool {
			return req.GetClient().GetId() == cl.GetId()
		})).Return(&protocol.RegisterIdentityResponse{
			Token: token,
			Apex:  testApex,
		}, nil)

		t1.Identify = cl
	}

	client, t2, assertion := setupClient(t, as, ctx, logger, cl, token, cfg, nil, m, false, 1)
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
	rpc.Receive(conn, status)
	as.Equal(protocol.TunnelStatusCode_STATUS_OK, status.GetStatus())

	buf := make([]byte, 2)
	n, err := io.ReadFull(conn, buf)
	as.NoError(err)
	as.Equal(2, n)
	as.Equal("hi", string(buf))
}

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

	cfg := &Config{
		path:     file.Name(),
		router:   skipmap.NewString[*url.URL](),
		Apex:     testApex,
		ClientID: cl.GetId(),
		Token:    string(token.GetToken()),
		Tunnels: []Tunnel{
			{
				Target: ts.URL,
			},
		},
	}
	as.NoError(cfg.validate())

	client, t2, assertion := setupClient(t, as, ctx, logger, cl, token, cfg, nil, nil, false, 1)
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

	cfg := &Config{
		path:     file.Name(),
		router:   skipmap.NewString[*url.URL](),
		Apex:     testApex,
		ClientID: cl.GetId(),
		Token:    string(token.GetToken()),
		Tunnels: []Tunnel{
			{
				Target: target,
			},
		},
	}
	as.NoError(cfg.validate())

	client, t2, assertion := setupClient(t, as, ctx, logger, cl, token, cfg, nil, nil, false, 1)
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

	cfg := &Config{
		path:     file.Name(),
		router:   skipmap.NewString[*url.URL](),
		Apex:     testApex,
		ClientID: cl.GetId(),
		Token:    string(token.GetToken()),
		Tunnels: []Tunnel{
			{
				Target: target,
			},
		},
	}
	as.NoError(cfg.validate())

	client, t2, assertion := setupClient(t, as, ctx, logger, cl, token, cfg, nil, nil, false, 1)
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
	rpc.Receive(conn, status)
	as.Equal(protocol.TunnelStatusCode_STATUS_OK, status.GetStatus())

	buf := make([]byte, 2)
	n, err := io.ReadFull(conn, buf)
	as.NoError(err)
	as.Equal(2, n)
	as.Equal("hi", string(buf))
}
