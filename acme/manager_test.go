package acme

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.miragespace.co/specter/kv/memory"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/cipher"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/testcond"

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

func getTCPListener(as *require.Assertions) (net.Listener, int) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	as.NoError(err)

	return l, l.Addr().(*net.TCPAddr).Port
}

func validateCert(as *require.Assertions, port int, serverName string) {
	err := testcond.WaitForCondition(func() bool {
		conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: true,
		})
		if err != nil {
			return false
		}
		if conn.Handshake() != nil {
			return false
		}
		cs := conn.ConnectionState()
		for _, cert := range cs.PeerCertificates {
			for _, name := range cert.DNSNames {
				if name == serverName {
					return true
				}
			}
		}
		return false
	}, time.Second, time.Second*15)
	as.NoError(err)
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

	listener, port := getTCPListener(as)
	gwConf := cipher.GetGatewayTLSConfig(manager.GetCertificate, []string{tun.ALPN(protocol.Link_UNKNOWN)})
	tlsListener := tls.NewListener(listener, gwConf)
	defer tlsListener.Close()

	go func() {
		for {
			conn, err := tlsListener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				b := make([]byte, 1)
				conn.Read(b)
			}(conn)
		}
	}()

	// managed domain
	err = manager.Initialize(ctx)
	as.NoError(err)

	validateCert(as, port, testManagedDomain)

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

	validateCert(as, port, testDynamicDomain)

	// test clean endpoint
	handler := AcmeManagerHandler(manager)
	req := httptest.NewRequest("POST", "/clean", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusNoContent, w.Result().StatusCode)
}
