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

	"github.com/zllovesuki/specter/spec/mocks"
	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/tun"

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

func getListener(as *require.Assertions) (net.Listener, int) {
	l, err := tls.Listen("tcp", "127.0.0.1:0", generateTLSConfig([]string{
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	}))
	as.Nil(err)

	return l, l.Addr().(*net.TCPAddr).Port
}

func getDialer(proto string, sn string) *tls.Dialer {
	return &tls.Dialer{
		Config: &tls.Config{
			ServerName:         fmt.Sprintf("%s.%s", sn, testDomain),
			InsecureSkipVerify: true,
			NextProtos:         []string{proto},
		},
	}
}

func getClient(host string, port int) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return getDialer(tun.ALPN(protocol.Link_HTTP), host).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
			},
		},
	}
}

func getStuff(as *require.Assertions) (int, *mocks.TunServer, func()) {
	l, port := getListener(as)

	mockS := new(mocks.TunServer)

	logger, err := zap.NewDevelopment()
	as.Nil(err)

	conf := GatewayConfig{
		Logger:      logger,
		Tun:         mockS,
		Listener:    l,
		RootDomain:  testDomain,
		GatewayPort: port,
	}

	g, err := New(conf)
	as.Nil(err)

	as.Equal(fmt.Sprintf("%s:%d", testDomain, port), g.GatewayConfig.RootDomain)

	ctx, cancel := context.WithCancel(context.Background())
	go g.Start(ctx)

	return port, mockS, func() {
		cancel()
		l.Close()
	}
}

func TestHTTPNotFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	c := getClient(testHost, port)

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, testDomain))
	as.Nil(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.Nil(err)

	as.Contains(string(b), "not found")

	mockS.AssertExpectations(t)
}

func TestHTTPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	testResponse := "this is fine"

	c1, c2 := net.Pipe()

	go func() {
		rd := bufio.NewReader(c2)
		_, err := http.ReadRequest(rd)
		as.Nil(err)
		resp := &http.Response{
			StatusCode:    200,
			Body:          io.NopCloser(bytes.NewBufferString(testResponse)),
			ContentLength: int64(len(testResponse)),
		}
		err = resp.Write(c2)
		as.Nil(err)
	}()

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_HTTP && l.GetHostname() == testHost
	})).Return(c1, nil)

	c := getClient(testHost, port)

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, testDomain))
	as.Nil(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.Nil(err)

	as.Contains(string(b), testResponse)

	mockS.AssertExpectations(t)
}

func TestTCPNotFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(nil, tun.ErrDestinationNotFound)

	dialer := getDialer(tun.ALPN(protocol.Link_TCP), testHost)
	_, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	as.Nil(err)

	<-time.After(time.Millisecond * 100)

	mockS.AssertExpectations(t)
}

func TestTCPFound(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	testHost := "hello"
	bufLength := 20

	c1, c2 := net.Pipe()

	go func() {
		buf := make([]byte, bufLength)
		n, err := io.ReadFull(c2, buf)
		as.Nil(err)
		as.Equal(bufLength, n)

		rand.Read(buf)
		n, err = c2.Write(buf)
		as.Nil(err)
		as.Equal(bufLength, n)
	}()

	mockS.On("Dial", mock.Anything, mock.MatchedBy(func(l *protocol.Link) bool {
		return l.GetAlpn() == protocol.Link_TCP && l.GetHostname() == testHost
	})).Return(c1, nil)

	dialer := getDialer(tun.ALPN(protocol.Link_TCP), testHost)
	conn, err := dialer.DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	as.Nil(err)

	buf := make([]byte, bufLength)

	rand.Read(buf)
	n, err := conn.Write(buf)
	as.Nil(err)
	as.Equal(bufLength, n)

	n, err = io.ReadFull(conn, buf)
	as.Nil(err)
	as.Equal(bufLength, n)

	mockS.AssertExpectations(t)
}
