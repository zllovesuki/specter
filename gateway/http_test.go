package gateway

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/iangudger/memnet"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHTTPRedirect(t *testing.T) {
	as := require.New(t)

	testHost := "http"
	testPath := "/sup"

	httpListener, httpPort := getTCPListener(as)
	port, _, _, done := setupGateway(t, as, httpListener)
	defer done()

	c := getHTTPClient(httpPort)
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s.%s%s", testHost, testDomain, testPath), nil)
	as.NoError(err)

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	re := resp.Header.Get("location")
	as.Equal(fmt.Sprintf("https://%s.%s:%d%s", testHost, testDomain, port, testPath), re)
	as.Equal(http.StatusMovedPermanently, resp.StatusCode)
}

func TestHTTPConnectProxy(t *testing.T) {
	as := require.New(t)

	httpListener, httpPort := getTCPListener(as)
	_, _, mockS, done := setupGateway(t, as, httpListener)
	defer done()

	testHost := "hello"
	bufLength := 16

	c1, c2 := memnet.NewBufferedStreamConnPair()

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

	// HTTP Connect start
	dialer := &net.Dialer{
		Timeout: time.Second,
	}
	conn, err := dialer.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", httpPort))
	as.NoError(err)

	proxyAddr := fmt.Sprintf("%s.%s:1234", testHost, testDomain) // port doesn't matter
	req := &http.Request{
		Method: http.MethodConnect,
		URL: &url.URL{
			Opaque: proxyAddr,
		},
		Host:   proxyAddr,
		Header: make(http.Header),
	}
	as.NoError(req.Write(conn))
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	as.NoError(err)
	as.Equal(http.StatusOK, resp.StatusCode)
	// HTTP Connect end

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
