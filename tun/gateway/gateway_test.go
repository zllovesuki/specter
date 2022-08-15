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

	"kon.nect.sh/specter/overlay"
	"kon.nect.sh/specter/spec/cipher"
	mocks "kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/tun"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

func getQuicDialer(proto string, sn string) func(context.Context, string) (quic.EarlyConnection, error) {
	return func(ctx context.Context, addr string) (quic.EarlyConnection, error) {
		return quic.DialAddrEarlyContext(ctx, addr, getDialer(proto, sn).Config, nil)
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
		Transport: &http3.RoundTripper{
			TLSClientConfig: getDialer("h3", host).Config,
			Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
				return quic.DialAddrEarlyContext(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsCfg, cfg)
			},
		},
	}
}

func getStuff(as *require.Assertions) (int, *mocks.TunServer, func()) {
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	h2, port := getH2Listener(as)

	ss := generateTLSConfig([]string{})
	alpnMux, err := overlay.NewMux(fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

	h3 := alpnMux.With(cipher.GetGatewayTLSConfig(func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &ss.Certificates[0], nil
	}, nil), append(cipher.H3Protos, tun.ALPN(protocol.Link_TCP))...)

	mockS := new(mocks.TunServer)

	conf := GatewayConfig{
		Logger:      logger,
		Tun:         mockS,
		H2Listener:  h2,
		H3Listener:  h3,
		RootDomain:  testDomain,
		GatewayPort: port,
		AdminUser:   os.Getenv("INTERNAL_USER"),
		AdminPass:   os.Getenv("INTERNAL_PASS"),
		StatsHandler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	g := New(conf)

	ctx, cancel := context.WithCancel(context.Background())
	go g.Start(ctx)
	go alpnMux.Accept(ctx)

	return port, mockS, func() {
		cancel()
		h2.Close()
		h3.Close()
		alpnMux.Close()
		g.Close()
	}
}

func TestH2HTTPNotFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	c := getH2Client(testHost, port)

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

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	c := getH3Client(testHost, port)

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

func serveMiniClient(ch chan net.Conn, resp string) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(resp))
	})
	h2s := &http2.Server{}
	h1 := &http.Server{
		Handler: h2c.NewHandler(h, h2s),
	}
	h1.Serve(&miniClient{c: ch})
}

func TestH1HTTPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	testResponse := "this is fine"

	c1, c2 := net.Pipe()
	ch := make(chan net.Conn, 1)
	go serveMiniClient(ch, testResponse)
	defer close(ch)
	ch <- c2

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH1Client(testHost, port)

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

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	testResponse := "this is fine"

	c1, c2 := net.Pipe()
	ch := make(chan net.Conn, 1)
	go serveMiniClient(ch, testResponse)
	defer close(ch)
	ch <- c2

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH2Client(testHost, port)

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

func TestH3HTTPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	testResponse := "this is fine from h3"

	c1, c2 := net.Pipe()
	ch := make(chan net.Conn, 1)
	go serveMiniClient(ch, testResponse)
	defer close(ch)
	ch <- c2

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH3Client(testHost, port)

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
	as.Equal("true", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH2TCPNotFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	dialer := getDialer(tun.ALPN(protocol.Link_TCP), testHost)
	stream, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

	status := &protocol.TunnelStatus{}
	err = rpc.Send(stream, status)
	as.NoError(err)
	err = rpc.Receive(stream, status)
	as.NoError(err)
	as.False(status.Ok)

	<-time.After(time.Millisecond * 100)

	mockS.AssertExpectations(t)
}

func TestH3TCPNotFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	dial := getQuicDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", port))
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
	as.False(status.Ok)

	<-time.After(time.Millisecond * 100)

	mockS.AssertExpectations(t)
}

func TestH2TCPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	bufLength := 20

	c1, c2 := net.Pipe()

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

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(c1, nil)

	dialer := getDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

	status := &protocol.TunnelStatus{}
	err = rpc.Send(conn, status)
	as.NoError(err)
	err = rpc.Receive(conn, status)
	as.NoError(err)
	as.True(status.Ok)

	buf := make([]byte, bufLength)

	rand.Read(buf)
	n, err := conn.Write(buf)
	as.NoError(err)
	as.Equal(bufLength, n)

	n, err = io.ReadFull(conn, buf)
	as.NoError(err)
	as.Equal(bufLength, n)

	mockS.AssertExpectations(t)
}

func TestH3TCPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	bufLength := 20

	c1, c2 := net.Pipe()

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

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(c1, nil)

	dial := getQuicDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", port))
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
	as.True(status.Ok)

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

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	dialer := getDialer("h3", testHost)
	_, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	as.Error(err)

	mockS.AssertExpectations(t)
}

func TestH3RejectALPN(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	dial := getQuicDialer(tun.ALPN(protocol.Link_HTTP2), testHost)
	_, err := dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", port))
	as.Error(err)

	mockS.AssertExpectations(t)
}
