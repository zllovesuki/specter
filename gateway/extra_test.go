package gateway

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	extraRootDomain = "x.y.z.net"
)

func TestH2ExtraApexIndex(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil, withExtraRootDomains([]string{extraRootDomain}))
	defer done()

	c := getH2Client("", tcpPort)
	dialer := getDialer(tun.ALPN(protocol.Link_HTTP2), "")
	dialer.Config.ServerName = extraRootDomain
	c.Transport.(*http.Transport).DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	}

	resp, err := c.Get(fmt.Sprintf("https://%s/", extraRootDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), extraRootDomain)
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("false", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH3ExtraApexIndex(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil, withExtraRootDomains([]string{extraRootDomain}))
	defer done()

	c := getH3Client("", udpPort)
	dialer := getDialer("h3", "")
	dialer.Config.ServerName = extraRootDomain
	c.Transport.(*http3.RoundTripper).TLSClientConfig = dialer.Config

	resp, err := c.Get(fmt.Sprintf("https://%s/", extraRootDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), extraRootDomain)
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("true", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH2HTTPNotFoundExtra(t *testing.T) {
	as := require.New(t)

	_, tcpPort, mockS, done := setupGateway(t, as, nil, withExtraRootDomains([]string{extraRootDomain}))
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
	dialer := getDialer(tun.ALPN(protocol.Link_HTTP2), "")
	dialer.Config.ServerName = testHost + "." + extraRootDomain
	c.Transport.(*http.Transport).DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	}

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, extraRootDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), "not found")
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("false", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestH3HTTPNotFoundExtra(t *testing.T) {
	as := require.New(t)

	udpPort, _, mockS, done := setupGateway(t, as, nil, withExtraRootDomains([]string{extraRootDomain}))
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
	dialer := getDialer("h3", "")
	dialer.Config.ServerName = testHost + "." + extraRootDomain
	c.Transport.(*http3.RoundTripper).TLSClientConfig = dialer.Config

	resp, err := c.Get(fmt.Sprintf("https://%s.%s", testHost, extraRootDomain))
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), "not found")
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("true", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}
