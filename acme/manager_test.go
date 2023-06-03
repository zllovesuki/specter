package acme

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"kon.nect.sh/specter/kv/memory"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util/testcond"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

var (
	_, b, _, _ = runtime.Caller(0)
	basepath   = filepath.Dir(b)
)

const (
	devAcme = "https://localhost:14001/dir"
)

type handshakeHook struct {
	mock.Mock
}

func (h *handshakeHook) onHandshake(sni string) {
	h.Called(sni)
}

func TestIntegrationACME(t *testing.T) {
	if os.Getenv("GO_INTEGRATION_ACME") == "" {
		t.Skip("skipping integration tests")
	}

	as := require.New(t)
	logger := zaptest.NewLogger(t)

	devPath := path.Join(basepath, "..", "dev", "pebble")
	devCa, err := os.ReadFile(path.Join(devPath, "certs", "cert.pem"))
	as.NoError(err)
	devTrustedRoots := x509.NewCertPool()
	devTrustedRoots.AppendCertsFromPEM(devCa)

	testcond.WaitForCondition(func() bool {
		tp := http.DefaultTransport.(*http.Transport).Clone()
		tp.TLSClientConfig = &tls.Config{
			RootCAs: devTrustedRoots,
		}
		client := &http.Client{
			Transport: tp,
		}
		req, err := http.NewRequest("GET", devAcme, nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		return resp.StatusCode == http.StatusOK
	}, time.Second, time.Second*15)

	// test hook
	hook := new(handshakeHook)
	defer hook.AssertExpectations(t)

	hook.On("onHandshake", testManagedDomain)
	hook.On("onHandshake", testDynamicDomain)

	// test manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kv := memory.WithHashFn(chord.Hash)

	solver := &ChordSolver{
		KV:             kv,
		ManagedDomains: []string{testManagedDomain},
	}

	manager, err := NewManager(ctx, ManagerConfig{
		Logger:           logger,
		KV:               kv,
		Email:            testEmail,
		DNSSolver:        solver,
		ManagedDomains:   []string{testManagedDomain},
		CA:               devAcme,
		testTrustedRoots: devTrustedRoots,
	})
	as.NoError(err)

	manager.OnHandshake(hook.onHandshake)

	// managed domain
	err = manager.Initialize(ctx)
	as.NoError(err)

	err = testcond.WaitForCondition(func() bool {
		fakeConn, _ := net.Pipe()
		cert, err := manager.GetCertificate(&tls.ClientHelloInfo{
			ServerName: testManagedDomain,
			Conn:       fakeConn,
		})
		if err != nil {
			return false
		}
		if cert == nil {
			return false
		}
		for _, name := range cert.Leaf.DNSNames {
			if name == testManagedDomain {
				return true
			}
		}
		return false
	}, time.Second, time.Second*15)
	as.NoError(err)

	// dynamic domain
	err = tun.SaveCustomHostname(ctx, kv, testDynamicDomain, &protocol.CustomHostname{
		ClientIdentity: &protocol.Node{
			Id:      chord.Random(),
			Address: "random",
		},
		ClientToken: &protocol.ClientToken{
			Token: []byte("random"),
		},
	})
	as.NoError(err)

	err = testcond.WaitForCondition(func() bool {
		fakeConn, _ := net.Pipe()
		cert, err := manager.GetCertificate(&tls.ClientHelloInfo{
			ServerName: testDynamicDomain,
			Conn:       fakeConn,
		})
		if err != nil {
			return false
		}
		if cert == nil {
			return false
		}
		for _, name := range cert.Leaf.DNSNames {
			if name == testDynamicDomain {
				return true
			}
		}
		return false
	}, time.Second, time.Second*15)
	as.NoError(err)

	// test clean endpoint
	handler := AcmeManagerHandler(manager)
	req := httptest.NewRequest("POST", "/clean", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusNoContent, w.Result().StatusCode)
}
