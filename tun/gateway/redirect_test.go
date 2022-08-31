package gateway

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPRedirect(t *testing.T) {
	as := require.New(t)

	testHost := "http"
	testPath := "/sup"

	httpListener, httpPort := getTCPListener(as)
	_, _, done := setupGateway(as, httpListener)
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
	as.Equal(fmt.Sprintf("https://%s.%s%s", testHost, testDomain, testPath), re)
	as.Equal(http.StatusMovedPermanently, resp.StatusCode)
}
