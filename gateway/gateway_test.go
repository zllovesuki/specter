package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"go.miragespace.co/specter/overlay"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/cipher"
	mocks "go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/bufconn"

	"github.com/go-chi/chi/v5"
	"github.com/libp2p/go-yamux/v4"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

const (
	testDomain = "a.b.c.d.com"
)

func generateTLSConfig(protos []string) *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         protos,
	}
}

func getTCPListener(as *require.Assertions) (net.Listener, int) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	as.NoError(err)

	return l, l.Addr().(*net.TCPAddr).Port
}

func getUDPListener(as *require.Assertions) (net.PacketConn, int) {
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	as.NoError(err)

	return l, l.LocalAddr().(*net.UDPAddr).Port
}

func getH2Listener(as *require.Assertions) (net.Listener, int) {
	l, err := tls.Listen("tcp", "127.0.0.1:0", generateTLSConfig([]string{
		tun.ALPN(protocol.Link_HTTP2),
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	}))
	as.NoError(err)

	return l, l.Addr().(*net.TCPAddr).Port
}

func getDialer(proto string, sn string) *tls.Dialer {
	sni := testDomain
	if sn != "" {
		sni = sn + "." + testDomain
	}
	dialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		},
	}
	if proto != "" {
		dialer.Config.NextProtos = []string{proto}
	}
	return dialer
}

func getQuicDialer(proto string, sn string) func(context.Context, string) (*quic.Conn, error) {
	return func(ctx context.Context, addr string) (*quic.Conn, error) {
		return quic.DialAddr(ctx, addr, getDialer(proto, sn).Config, nil)
	}
}

func getH1Client(host string, port int) *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2: false,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return getDialer("http/1.1", host).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
			},
		},
	}
}

func getHTTPClient(port int) *http.Client {
	dialer := &net.Dialer{}
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
			},
		},
	}
}

func getH2Client(host string, port int) *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return getDialer(tun.ALPN(protocol.Link_HTTP2), host).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
			},
		},
	}
}

func getH3Client(host string, port int) *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http3.Transport{
			TLSClientConfig: getDialer("h3", host).Config,
			Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
				return quic.DialAddr(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsCfg, cfg)
			},
		},
	}
}

type configOption func(*GatewayConfig)

func withExtraRootDomains(domains []string) configOption {
	return func(gc *GatewayConfig) {
		gc.RootDomains = append(gc.RootDomains, domains...)
	}
}

func setupGateway(t *testing.T, as *require.Assertions, httpListener net.Listener, options ...configOption) (udpPort int, tcpPort int, mockS *mocks.TunnelServer, done func()) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	var q net.PacketConn
	var h2 net.Listener

	q, udpPort = getUDPListener(as)

	h2, tcpPort = getH2Listener(as)

	ss := generateTLSConfig([]string{})
	alpnMux, err := overlay.NewMux(&quic.Transport{Conn: q})
	as.NoError(err)

	h3 := alpnMux.With(cipher.GetGatewayTLSConfig(func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &ss.Certificates[0], nil
	}, nil), append(cipher.H3Protos, tun.ALPN(protocol.Link_TCP))...)

	mockS = new(mocks.TunnelServer)

	fakeStats := chi.NewRouter()
	fakeStats.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	conf := GatewayConfig{
		Logger:       logger,
		TunnelServer: mockS,
		HTTPListener: httpListener,
		H2Listener:   h2,
		H3Listener:   h3,
		RootDomains:  []string{testDomain},
		GatewayPort:  udpPort,
		AdminUser:    os.Getenv("INTERNAL_USER"),
		AdminPass:    os.Getenv("INTERNAL_PASS"),
		Handlers: InternalHandlers{
			Chord: fakeStats,
		},
		Options: Options{
			TransportBufferSize: 1024 * 8,
			ProxyBufferSize:     1024 * 8,
		},
	}

	for _, option := range options {
		option(&conf)
	}

	g := New(conf)

	ctx, cancel := context.WithCancel(context.Background())
	go alpnMux.Accept(ctx)
	g.MustStart(ctx)

	return udpPort, tcpPort, mockS, func() {
		cancel()
		h2.Close()
		h3.Close()
		alpnMux.Close()
		g.Close()
	}
}

func TestH2HTTPNotFound(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	mockS.On("Identity").Return(&protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	})
	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	c := getH2Client(testHost, tcpPort)

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, testDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), "not found")
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("false", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH3HTTPNotFound(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	mockS.On("Identity").Return(&protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	})
	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	c := getH3Client(testHost, udpPort)

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, testDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), "not found")
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("true", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestHTTPNotConnected(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	mockS.On("Identity").Return(&protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	})
	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrTunnelClientNotConnected)

	c := getH3Client(testHost, udpPort)

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, testDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), "not connected")
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("true", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

type miniClient struct {
	c chan net.Conn
	net.Listener
}

func (b *miniClient) Accept() (net.Conn, error) {
	c := <-b.c
	if c == nil {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (b *miniClient) Close() error {
	return nil
}

func serveMiniClient(as *require.Assertions, ch chan net.Conn, resp string) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		as.Equal("https", r.Header.Get("X-Forwarded-Proto"))
		as.NotEmpty(r.Header.Get("X-Forwarded-Host"))
		w.Write([]byte(resp))
	})
	h2s := &http2.Server{}
	h1 := &http.Server{
		Handler: h2c.NewHandler(h, h2s),
	}
	h1.Serve(&miniClient{c: ch})
}

func TestRejectInvalidHostnames(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	conf := GatewayConfig{
		Logger:      logger,
		RootDomains: []string{testDomain},
		AdminUser:   os.Getenv("INTERNAL_USER"),
		AdminPass:   os.Getenv("INTERNAL_PASS"),
	}
	g := New(conf)

	hostnames := []string{
		"bleh",
		"bleh.com",
		"192.168.1.1",
	}

	for _, hostname := range hostnames {
		_, _, err := g.parseAddr(hostname)
		as.Error(err)
	}
}

func TestH1HTTPFound(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"
	testResponse := "this is fine"

	c1, c2 := bufconn.BufferedPipe(8192)
	ch := make(chan net.Conn, 1)
	go serveMiniClient(as, ch, testResponse)
	defer close(ch)
	ch <- c2

	mockS.On("Identity").Return(&protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	})
	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH1Client(testHost, tcpPort)

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s.%s", testHost, testDomain), nil)
	as.NoError(err)

	req.Host = "fail.com"

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), testResponse)
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("false", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH2HTTPFound(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"
	testResponse := "this is fine"

	c1, c2 := bufconn.BufferedPipe(8192)
	ch := make(chan net.Conn, 1)
	go serveMiniClient(as, ch, testResponse)
	defer close(ch)
	ch <- c2

	mockS.On("Identity").Return(&protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	})
	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH2Client(testHost, tcpPort)

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s.%s", testHost, testDomain), nil)
	as.NoError(err)

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), testResponse)
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("false", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH3HTTPFound(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"
	testResponse := "this is fine from h3"

	c1, c2 := bufconn.BufferedPipe(8192)
	ch := make(chan net.Conn, 1)
	go serveMiniClient(as, ch, testResponse)
	defer close(ch)
	ch <- c2

	mockS.On("Identity").Return(&protocol.Node{
		Id:      chord.Random(),
		Address: "127.0.0.1:1234",
	})
	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH3Client(testHost, udpPort)

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s.%s", testHost, testDomain), nil)
	as.NoError(err)

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), testResponse)
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("true", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH2TCPNotFound(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	dialer := getDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	as.NoError(err)

	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	session, err := yamux.Client(conn, cfg, nil)
	as.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := session.OpenStream(ctx)
	as.NoError(err)

	status := &protocol.TunnelStatus{}
	err = rpc.Send(stream, status)
	as.NoError(err)
	err = rpc.Receive(stream, status)
	as.NoError(err)
	as.NotEqual(protocol.TunnelStatusCode_STATUS_OK, status.GetStatus())

	<-time.After(time.Millisecond * 100)

	mockS.AssertExpectations(t)
}

func TestH3TCPNotFound(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	dial := getQuicDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", udpPort))
	as.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := conn.OpenStreamSync(ctx)
	as.NoError(err)

	status := &protocol.TunnelStatus{}
	err = rpc.Send(b, status)
	as.NoError(err)
	err = rpc.Receive(b, status)
	as.NoError(err)
	as.NotEqual(protocol.TunnelStatusCode_STATUS_OK, status.GetStatus())

	<-time.After(time.Millisecond * 100)

	mockS.AssertExpectations(t)
}

func TestTCPNotConnected(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrTunnelClientNotConnected)

	dial := getQuicDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", udpPort))
	as.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := conn.OpenStreamSync(ctx)
	as.NoError(err)

	status := &protocol.TunnelStatus{}
	err = rpc.Send(b, status)
	as.NoError(err)
	err = rpc.Receive(b, status)
	as.NoError(err)
	as.Equal(protocol.TunnelStatusCode_NO_DIRECT, status.GetStatus())

	<-time.After(time.Millisecond * 100)

	mockS.AssertExpectations(t)
}

func TestH2TCPFound(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"
	bufLength := 20

	c1, c2 := bufconn.BufferedPipe(8192)

	go func() {
		tun.SendStatusProto(c2, nil)

		buf := make([]byte, bufLength)
		n, err := io.ReadFull(c2, buf)
		as.NoError(err)
		as.Equal(bufLength, n)

		rand.Read(buf)
		n, err = c2.Write(buf)
		as.NoError(err)
		as.Equal(bufLength, n)
	}()

	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(c1, nil)

	dialer := getDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	as.NoError(err)

	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	session, err := yamux.Client(conn, cfg, nil)
	as.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := session.OpenStream(ctx)
	as.NoError(err)

	status := &protocol.TunnelStatus{}
	err = rpc.Send(stream, status)
	as.NoError(err)
	err = rpc.Receive(stream, status)
	as.NoError(err)
	as.Equal(protocol.TunnelStatusCode_STATUS_OK, status.GetStatus())

	buf := make([]byte, bufLength)

	rand.Read(buf)
	n, err := stream.Write(buf)
	as.NoError(err)
	as.Equal(bufLength, n)

	n, err = io.ReadFull(stream, buf)
	as.NoError(err)
	as.Equal(bufLength, n)

	mockS.AssertExpectations(t)
}

func TestH3TCPFound(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"
	bufLength := 20

	c1, c2 := bufconn.BufferedPipe(8192)

	go func() {
		tun.SendStatusProto(c2, nil)

		buf := make([]byte, bufLength)
		n, err := io.ReadFull(c2, buf)
		as.NoError(err)
		as.Equal(bufLength, n)

		rand.Read(buf)
		n, err = c2.Write(buf)
		as.NoError(err)
		as.Equal(bufLength, n)
	}()

	mockS.On("DialClient", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(c1, nil)

	dial := getQuicDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", udpPort))
	as.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, err := conn.OpenStreamSync(ctx)
	as.NoError(err)

	status := &protocol.TunnelStatus{}
	err = rpc.Send(stream, status)
	as.NoError(err)
	err = rpc.Receive(stream, status)
	as.NoError(err)
	as.Equal(protocol.TunnelStatusCode_STATUS_OK, status.GetStatus())

	buf := make([]byte, bufLength)

	rand.Read(buf)
	n, err := stream.Write(buf)
	as.NoError(err)
	as.Equal(bufLength, n)

	n, err = io.ReadFull(stream, buf)
	as.NoError(err)
	as.Equal(bufLength, n)

	mockS.AssertExpectations(t)
}

func TestH2RejectALPN(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	dialer := getDialer("h3", testHost)
	_, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	as.Error(err)

	mockS.AssertExpectations(t)
}

func TestH3RejectALPN(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil)
	defer done()

	testHost := "hello"

	dial := getQuicDialer(tun.ALPN(protocol.Link_HTTP2), testHost)
	_, err := dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", udpPort))
	as.Error(err)

	mockS.AssertExpectations(t)
}
