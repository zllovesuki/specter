package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/rtt"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/acceptor"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

const (
	testHostname = "abcd"
	testApex     = "specter.dev"
)

func TestMain(m *testing.M) {
	checkInterval = time.Millisecond * 200
	rttInterval = time.Millisecond * 250
	m.Run()
}

func setupRPC(ctx context.Context,
	logger *zap.Logger,
	s protocol.TunnelService,
	router *transport.StreamRouter,
	acc *acceptor.HTTP2Acceptor,
) {
	tunTwirp := protocol.NewTunnelServiceServer(s, twirp.WithServerHooks(&twirp.ServerHooks{
		RequestRouted: func(ctx context.Context) (context.Context, error) {
			delegation := rpc.GetDelegation(ctx)
			if delegation.Certificate == nil {
				return ctx, fmt.Errorf("missing client certificate")
			}
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
	rpcHandler.Mount(tunTwirp.PathPrefix(), tunTwirp)

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

func setupFakeNodes(rr *mocks.Measurement, s *mocks.TunnelService, expectGenerate bool) []*protocol.Node {
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
	if expectGenerate {
		s.On("GenerateHostname", mock.Anything, mock.Anything).Return(&protocol.GenerateHostnameResponse{
			Hostname: testHostname,
		}, nil)
	}

	return fakeNodes
}

func setupClient(
	t *testing.T,
	as *require.Assertions,
	ctx context.Context,
	logger *zap.Logger,
	pkiClient *mocks.PKIClient,
	cfg *Config,
	reload <-chan os.Signal,
	m func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call),
	preference bool,
	times int,
) (
	*Client,
	*mocks.MemoryTransport,
	func(),
) {
	t1, t2 := mocks.PipeTransport()
	rr := new(mocks.Measurement)

	s := new(mocks.TunnelService)
	router := transport.NewStreamRouter(logger, nil, t2)
	acc := acceptor.NewH2Acceptor(nil)

	expectGenerate := true
	if cfg.Tunnels[0].Hostname != "" {
		expectGenerate = false
	}
	fakeNodes := setupFakeNodes(rr, s, expectGenerate)

	var publishCall *mock.Call
	if preference {
		publishCall = s.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
			// ensure that the node with the lowest latency is the first preference
			preferenceMatched := len(req.GetServers()) > 0 && req.GetServers()[0].GetAddress() == fakeNodes[1].GetAddress()
			hostnameMatched := req.GetHostname() == testHostname
			return preferenceMatched && hostnameMatched
		})).Return(&protocol.PublishTunnelResponse{
			Published: fakeNodes,
		}, nil).Times(times)
	} else {
		publishCall = s.On("PublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.PublishTunnelRequest) bool {
			return req.GetHostname() == testHostname
		})).Return(&protocol.PublishTunnelResponse{
			Published: fakeNodes,
		}, nil).Times(times)
	}

	if m != nil {
		m(s, t1, publishCall)
	}

	go router.Accept(ctx)

	setupRPC(ctx, logger, s, router, acc)

	client, err := NewClient(rpc.DisablePooling(ctx), ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		PKIClient:       pkiClient,
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
		if pkiClient != nil {
			pkiClient.AssertExpectations(t)
		}
	}
}

func makeCertificate(as *require.Assertions, logger *zap.Logger, client *protocol.Node, token *protocol.ClientToken, privKey ed25519.PrivateKey) (certDer []byte, certPem, keyPem string) {
	// generate a CA
	caPubKey, caPrivKey, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"dev"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, caPubKey, caPrivKey)
	as.NoError(err)

	var (
		certPubKey  ed25519.PublicKey
		certPrivKey ed25519.PrivateKey
	)

	if privKey != nil {
		certPubKey = privKey.Public().(ed25519.PublicKey)
		certPrivKey = privKey
	} else {
		certPubKey, certPrivKey, err = ed25519.GenerateKey(rand.Reader)
		as.NoError(err)
	}

	der, err := pki.GenerateCertificate(logger, tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  caPrivKey,
	}, pki.IdentityRequest{
		Subject:   pki.MakeSubjectV2(client.GetId(), token.GetToken()),
		PublicKey: certPubKey,
	})
	as.NoError(err)

	x509PrivKey, err := x509.MarshalPKCS8PrivateKey(certPrivKey)
	if err != nil {
		panic(err)
	}
	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509PrivKey,
	})

	certDer = der
	certPem = string(pki.MarshalCertificate(der))
	keyPem = certPrivKeyPEM.String()
	return
}

func transportHelper(t *mocks.MemoryTransport, der []byte) {
	t.On("WithClientCertificate", mock.MatchedBy(func(cert tls.Certificate) bool {
		return bytes.Equal(der, cert.Certificate[0])
	})).Run(func(args mock.Arguments) {
		cert := args.Get(0).(tls.Certificate)
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			panic(err)
		}
		t.WithCertificate(parsed)
	}).Return(nil).Once()
}

func defaultNoHostnames(s *mocks.TunnelService) {
	resp := &protocol.RegisteredHostnamesResponse{
		Hostnames: make([]string, 0),
	}
	s.On("RegisteredHostnames", mock.Anything, mock.Anything).Return(resp, nil)
}
