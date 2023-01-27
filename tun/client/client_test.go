package client

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
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
	"kon.nect.sh/specter/util/httprate"
	"kon.nect.sh/specter/util/router"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func setupRPC(ctx context.Context,
	logger *zap.Logger,
	s protocol.TunnelService,
	router *router.StreamRouter,
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
	rpcHandler.Use(httprate.LimitByIP(10, time.Second))
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

func TestPublishPreferenceRTT(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	t1, t2 := mocks.PipeTransport()
	rr := new(mocks.Measurement)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := new(mocks.TunnelService)
	router := router.NewStreamRouter(logger, nil, t2)
	acc := acceptor.NewH2Acceptor(nil)
	defer acc.Close()

	token := &protocol.ClientToken{
		Token: []byte("test"),
	}
	cl := &protocol.Node{
		Id: chord.Random(),
	}
	node := &protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.2:4567",
	}
	apex := "127.0.0.1:1234"

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
			Apex: "127.0.0.1",
		}, nil)
	}

	s.On("Ping", mock.Anything, mock.Anything).Return(&protocol.ClientPingResponse{
		Node: node,
		Apex: "127.0.0.1",
	}, nil)
	s.On("GetNodes", mock.Anything, mock.Anything).Return(&protocol.GetNodesResponse{
		Nodes: fakeNodes,
	}, nil)
	s.On("GenerateHostname", mock.Anything, mock.Anything).Return(&protocol.GenerateHostnameResponse{
		Hostname: "abcd",
	}, nil)
	s.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
		// ensure that the node with the lowest latency is the first preference
		return len(req.GetServers()) > 0 && req.GetServers()[0].GetId() == fakeNodes[1].GetId()
	})).Return(&protocol.PublishTunnelResponse{
		Published: fakeNodes,
	}, nil)

	go router.Accept(ctx)

	setupRPC(ctx, logger, s, router, acc, token, cl)

	cfg := &Config{
		path:     file.Name(),
		router:   skipmap.NewString[*url.URL](),
		Apex:     apex,
		ClientID: cl.GetId(),
		Token:    string(token.GetToken()),
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:2345",
			},
		},
	}
	client, err := NewClient(rpc.DisablePooling(ctx), logger, t1, cfg, rr)
	as.NoError(err)

	as.NoError(client.Register(ctx))

	as.NoError(client.Initialize(ctx))
}
