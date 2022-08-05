package gateway

import (
	"bufio"
	"bytes"
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
	"testing"
	"time"

	"kon.nect.sh/specter/overlay"
	"kon.nect.sh/specter/spec/cipher"
	"kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
	return &tls.Dialer{
		Config: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
			NextProtos:         []string{proto},
		},
	}
}

func getQuicDialer(proto string, sn string) func(context.Context, string) (quic.EarlyConnection, error) {
	return func(ctx context.Context, addr string) (quic.EarlyConnection, error) {
		return quic.DialAddrEarlyContext(ctx, addr, getDialer(proto, sn).Config, nil)
	}
}

func getH2Client(host string, port int) *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return getDialer(tun.ALPN(protocol.Link_HTTP), host).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
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
	alpnMux, err := overlay.NewMux(logger, fmt.Sprintf("127.0.0.1:%d", port))
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
	}

	g, err := New(conf)
	as.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	go g.Start(ctx)
	go alpnMux.Accept(ctx)

	return port, mockS, func() {
		cancel()
		h2.Close()
		h3.Close()
		alpnMux.Close()
	}
}

func TestH2ApexIndex(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	c := getH2Client("", port)

	resp, err := c.Get(fmt.Sprintf("https://%s/", testDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), testDomain)
	as.NotEmpty(resp.Header.Get("alt-svc"))

	mockS.AssertExpectations(t)
}

func TestH3ApexIndex(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	c := getH3Client("", port)

	resp, err := c.Get(fmt.Sprintf("https://%s/", testDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), testDomain)
	as.NotEmpty(resp.Header.Get("alt-svc"))

	mockS.AssertExpectations(t)
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

	mockS.AssertExpectations(t)
}

func TestH2HTTPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	testResponse := "this is fine"

	c1, c2 := net.Pipe()

	go func() {
		rd := bufio.NewReader(c2)
		_, err := http.ReadRequest(rd)
		as.NoError(err)
		resp := &http.Response{
			StatusCode:    200,
			Body:          io.NopCloser(bytes.NewBufferString(testResponse)),
			ContentLength: int64(len(testResponse)),
		}
		err = resp.Write(c2)
		as.NoError(err)
	}()

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH2Client(testHost, port)

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, testDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), testResponse)
	as.NotEmpty(resp.Header.Get("alt-svc"))

	mockS.AssertExpectations(t)
}

func TestH3HTTPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	testResponse := "this is fine from h3"

	c1, c2 := net.Pipe()

	go func() {
		rd := bufio.NewReader(c2)
		_, err := http.ReadRequest(rd)
		as.NoError(err)
		resp := &http.Response{
			StatusCode:    200,
			Body:          io.NopCloser(bytes.NewBufferString(testResponse)),
			ContentLength: int64(len(testResponse)),
		}
		err = resp.Write(c2)
		as.NoError(err)
	}()

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getH3Client(testHost, port)

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, testDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), testResponse)
	as.NotEmpty(resp.Header.Get("alt-svc"))

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
	_, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

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

	_, err = b.Write([]byte("a"))
	as.NoError(err)

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
