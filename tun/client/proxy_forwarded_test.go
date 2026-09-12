package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.miragespace.co/specter/gateway"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/bufconn"
	"go.miragespace.co/specter/util/pipe"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap/zaptest"
)

type forwardedRequest struct {
	Host    string
	URI     string
	Headers http.Header
}

func captureForwardedRequest(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(forwardedRequest{Host: r.Host, URI: r.RequestURI, Headers: r.Header})
}

// Supply the same private stream entrypoint used by authenticated server
// delegations, while retaining actual HTTP serialization between both proxies.
type forwardedTunnelServer struct {
	client *Client
}

func (s *forwardedTunnelServer) DialClient(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	local, remote := bufconn.BufferedPipe(8192)
	if err := s.client.handleIncomingDelegation(ctx, link, forwardedPeerConn{local}); err != nil {
		remote.Close()
		return nil, err
	}
	return remote, nil
}

func (*forwardedTunnelServer) Identity() *protocol.Node {
	return &protocol.Node{Address: "192.0.2.20:443"}
}

func (*forwardedTunnelServer) DialInternal(context.Context, *protocol.Node) (net.Conn, error) {
	return nil, tun.ErrLookupFailed
}

type forwardedPeerConn struct{ net.Conn }

func (forwardedPeerConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("192.0.2.20"), Port: 443}
}

func TestGatewayClientForwardedContext(t *testing.T) {
	der, _, key := testMakeRSACert(require.New(t))
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"h2", "http/1.1", "h3"},
	}

	for _, tc := range []struct {
		target, mode, proto string
	}{
		{"http", "", "http/1.1"},
		{"http", "target", "h2"},
		{"http", "hostname", "http/1.1"},
		{"pipe", "custom", "h2"},
		{"pipe", "legacy-override", "http/1.1"},
	} {
		t.Run(tc.target+"/"+tc.mode+"/"+tc.proto, func(t *testing.T) {
			pipeTarget := tc.target == "pipe"
			mode, proto := tc.mode, tc.proto
			as := require.New(t)
			var target string
			if pipeTarget {
				path, address := randomPipeTarget(t, "forwarded-*")
				target = address
				listener, err := pipe.ListenPipe(path)
				as.NoError(err)
				server := &http.Server{Handler: http.HandlerFunc(captureForwardedRequest)}
				go server.Serve(listener)
				t.Cleanup(func() { server.Close() })
			} else {
				server := httptest.NewServer(http.HandlerFunc(captureForwardedRequest))
				target = server.URL
				t.Cleanup(server.Close)
			}

			// HTTP/1 routes by SNI; HTTP/2 routes by authority for coalescing.
			hostname := "tls.example.com"
			if proto == "h2" {
				hostname = "visitor.example.com"
			}
			tunnel := Tunnel{Hostname: hostname, Target: target, ProxyHeaderMode: mode}
			if mode == "custom" || mode == "legacy-override" {
				tunnel.ProxyHeaderHost = "backend.example.com:9000"
			}
			if mode == "legacy-override" {
				tunnel.ProxyHeaderMode = ""
			}
			cfg := &Config{router: skipmap.NewString[route](), Tunnels: []Tunnel{tunnel}}
			as.NoError(cfg.validate())
			cfg.buildRouter()
			c := &Client{
				ClientConfig: ClientConfig{Logger: zaptest.NewLogger(t), Configuration: cfg},
				forwarder: &forwarder{
					logger:     zaptest.NewLogger(t),
					rootDomain: atomic.NewString("example.com"),
					proxies:    skipmap.NewString[*httpProxy](),
				},
			}
			t.Cleanup(func() {
				c.proxies.Range(func(_ string, proxy *httpProxy) bool {
					proxy.acceptor.Close()
					proxy.forwarder.Close()
					return true
				})
			})

			listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
			as.NoError(err)
			t.Cleanup(func() { listener.Close() })
			quicListener, err := quic.ListenAddr("127.0.0.1:0", tlsConfig, nil)
			as.NoError(err)
			t.Cleanup(func() { quicListener.Close() })
			gw := gateway.New(gateway.GatewayConfig{
				Logger:       c.Logger,
				TunnelServer: &forwardedTunnelServer{client: c},
				H2Listener:   listener,
				H3Listener:   quicListener,
				GatewayPort:  8443,
				Options:      gateway.Options{TransportBufferSize: 8192, ProxyBufferSize: 8192},
			})
			gw.MustStart(t.Context())
			t.Cleanup(gw.Close)

			transport := &http.Transport{
				ForceAttemptHTTP2: proto == "h2",
				DialTLSContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					dialer := &tls.Dialer{Config: &tls.Config{
						InsecureSkipVerify: true,
						ServerName:         "tls.example.com",
						NextProtos:         []string{proto},
					}}
					return dialer.DialContext(ctx, "tcp", listener.Addr().String())
				},
			}
			t.Cleanup(transport.CloseIdleConnections)
			visitor := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			req, err := http.NewRequestWithContext(t.Context(), "GET", "https://visitor.example.com:8443/a%2Fb?q=one%2Ftwo", nil)
			as.NoError(err)
			req.Header = http.Header{
				"Forwarded":         {"for=198.51.100.1;proto=http;host=attacker.example"},
				"X-Forwarded-For":   {"198.51.100.1, 198.51.100.2", "198.51.100.3"},
				"X-Forwarded-Host":  {"attacker.example:1234", "second.example"},
				"X-Forwarded-Proto": {"http", "ftp"},
				"X-Real-Ip":         {"198.51.100.4"},
				"True-Client-Ip":    {"198.51.100.5"},
			}
			if proto == "http/1.1" {
				req.Header.Set("Connection", "X-Forwarded-For, x-forwarded-host, X-Forwarded-Proto")
			}
			resp, err := visitor.Do(req)
			as.NoError(err)
			defer resp.Body.Close()
			as.Equal(http.StatusOK, resp.StatusCode)
			as.Equal(proto == "h2", resp.ProtoMajor == 2)
			var received forwardedRequest
			as.NoError(json.NewDecoder(resp.Body).Decode(&received))

			expectedHost := cfg.Tunnels[0].parsed.Host
			if pipeTarget {
				expectedHost = "pipe"
			}
			if mode == "hostname" {
				expectedHost = hostname
			} else if mode == "custom" || mode == "legacy-override" {
				expectedHost = tunnel.ProxyHeaderHost
			}
			as.Equal(expectedHost, received.Host)
			as.Equal("/a%2Fb?q=one%2Ftwo", received.URI)
			as.Equal([]string{"127.0.0.1"}, received.Headers.Values("X-Forwarded-For"))
			as.Equal([]string{hostname + ":8443"}, received.Headers.Values("X-Forwarded-Host"))
			as.Equal([]string{"https"}, received.Headers.Values("X-Forwarded-Proto"))
			for _, header := range []string{"Forwarded", "X-Real-IP", "True-Client-IP", "Connection"} {
				as.Empty(received.Headers.Values(header), header)
			}
		})
	}
}
