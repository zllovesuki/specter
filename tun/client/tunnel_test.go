package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap/zaptest"
)

func TestPublishHostnameReuse(t *testing.T) {
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

	der, cert, key := makeCertificate(as, logger, cl, token, nil)
	cfg := &Config{
		path:        file.Name(),
		router:      skipmap.NewString[route](),
		Apex:        testApex,
		Certificate: cert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:5432",
			},
			{
				Target: "tcp://127.0.0.1:2345",
			},
			{
				Target: "https://example.com",
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		resp := &protocol.RegisteredHostnamesResponse{
			Hostnames: []string{testHostname, "bastion.example.com"},
		}
		s.On("RegisteredHostnames", mock.Anything, mock.Anything).Return(resp, nil)
		transportHelper(t1, der)
	}

	client, _, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, true, len(cfg.Tunnels))
	defer assertion()
	defer client.Close()

	// assert that we are not reusing the custom domain
	for _, tunnel := range client.Configuration.Tunnels {
		as.Equal(testHostname, tunnel.Hostname)
	}
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
		path:   file.Name(),
		router: skipmap.NewString[route](),
		Apex:   testApex,
		Tunnels: []Tunnel{
			{
				Target: fmt.Sprintf("tcp://%s", tcpListener.Addr()),
			},
		},
	}
	as.NoError(cfg.validate())

	key, err := pki.UnmarshalPrivateKey([]byte(cfg.PrivKey))
	as.NoError(err)
	der, cert, _ := makeCertificate(as, logger, cl, token, key)
	pkiClient := new(mocks.PKIClient)
	pkiClient.On("RequestCertificate", mock.Anything, mock.Anything).Return(&protocol.CertificateResponse{
		CertDer: der,
		CertPem: []byte(cert),
	}, nil).Once()

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		s.On("RegisterIdentity", mock.Anything, mock.Anything).Return(&protocol.RegisterIdentityResponse{
			Apex: testApex,
		}, nil)

		defaultNoHostnames(s)
		transportHelper(t1, der)

		t1.Identify = cl
	}

	client, t2, assertion := setupClient(t, as, ctx, logger, pkiClient, cfg, nil, m, false, 1)
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

func TestUnpublishTunnel(t *testing.T) {
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
				Hostname: testHostname,
				Target:   ts.URL,
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		s.On("UnpublishTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.UnpublishTunnelRequest) bool {
			return req.GetHostname() == testHostname
		})).Return(&protocol.UnpublishTunnelResponse{}, nil).NotBefore(publishCall)

		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, _, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, false, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	err = client.UnpublishTunnel(ctx, Tunnel{Hostname: testHostname})
	as.NoError(err)

	curr := client.GetCurrentConfig()
	as.Len(curr.Tunnels, 0)
}

func TestReleaseTunnel(t *testing.T) {
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
				Hostname: testHostname,
				Target:   ts.URL,
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		s.On("ReleaseTunnel", mock.Anything, mock.MatchedBy(func(req *protocol.ReleaseTunnelRequest) bool {
			return req.GetHostname() == testHostname
		})).Return(&protocol.ReleaseTunnelResponse{}, nil).NotBefore(publishCall)

		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, _, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, false, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	err = client.ReleaseTunnel(ctx, Tunnel{Hostname: testHostname})
	as.NoError(err)

	curr := client.GetCurrentConfig()
	as.Len(curr.Tunnels, 0)
}
