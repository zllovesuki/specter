package gateway

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testUser = "zzzAdminzzz"
	testPass = "yyyPasswordzzz"
)

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
	as.Equal("false", resp.Header.Get("http3"))

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
	as.Equal("true", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}

func TestInternalWithAuth(t *testing.T) {
	os.Setenv("INTERNAL_USER", testUser)
	os.Setenv("INTERNAL_PASS", testPass)
	defer func() {
		os.Setenv("INTERNAL_USER", "")
		os.Setenv("INTERNAL_PASS", "")
	}()

	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	c := getH2Client("", port)

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/_internal/stats", testDomain), nil)
	as.NoError(err)
	req.SetBasicAuth(testUser, testPass)

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	as.Equal(http.StatusOK, resp.StatusCode)
	as.NotEmpty(resp.Header.Get("alt-svc"))

	mockS.AssertExpectations(t)
}

func TestInternalNoAuth(t *testing.T) {
	os.Setenv("INTERNAL_USER", testUser)
	os.Setenv("INTERNAL_PASS", testPass)
	defer func() {
		os.Setenv("INTERNAL_USER", "")
		os.Setenv("INTERNAL_PASS", "")
	}()

	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	c := getH2Client("", port)

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/_internal/stats", testDomain), nil)
	as.NoError(err)

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	as.Equal(http.StatusUnauthorized, resp.StatusCode)
	as.NotEmpty(resp.Header.Get("alt-svc"))

	mockS.AssertExpectations(t)
}

func TestInternalDisabled(t *testing.T) {
	as := require.New(t)

	port, mockS, done := getStuff(as)
	defer done()

	c := getH2Client("", port)

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/_internal/stats", testDomain), nil)
	as.NoError(err)

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	as.Equal(http.StatusNotFound, resp.StatusCode)
	as.NotEmpty(resp.Header.Get("alt-svc"))

	mockS.AssertExpectations(t)
}
