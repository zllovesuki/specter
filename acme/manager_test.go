package acme

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
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

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

var (
	_, b, _, _ = runtime.Caller(0)
	basepath   = filepath.Dir(b)
)

func TestACME(t *testing.T) {
	if os.Getenv("GO_RUN_ACME") == "" {
		t.Skip("skipping integration tests")
	}

	as := require.New(t)
	logger := zaptest.NewLogger(t)

	pool, err := dockertest.NewPool("")
	as.NoError(err)

	err = pool.Client.Ping()
	as.NoError(err)

	// setup acme

	devPath := path.Join(basepath, "..", "dev", "pebble")

	devCa, err := os.ReadFile(path.Join(devPath, "certs", "cert.pem"))
	as.NoError(err)
	devTrustedRoots := x509.NewCertPool()
	devTrustedRoots.AppendCertsFromPEM(devCa)

	pebble, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "letsencrypt/pebble",
		Tag:        "latest",
		Env: []string{
			"PEBBLE_VA_NOSLEEP=1",
			"PEBBLE_VA_ALWAYS_VALID=1",
		},
		Mounts: []string{
			devPath + ":/pebble:ro",
		},
		Cmd: []string{
			"pebble",
			"-config",
			"/pebble/config.json",
		},
	})
	as.NoError(err)

	t.Cleanup(func() {
		pool.Purge(pebble)
	})

	pebblePort := pebble.GetPort("14000/tcp")

	pool.Retry(func() error {
		tp := http.DefaultTransport.(*http.Transport).Clone()
		tp.TLSClientConfig = &tls.Config{
			RootCAs: devTrustedRoots,
		}
		client := &http.Client{
			Transport: tp,
		}
		req, err := http.NewRequest("GET", fmt.Sprintf("https://localhost:%s/dir", pebblePort), nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("not ready")
		}
		return nil
	})

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
		CA:               fmt.Sprintf("https://localhost:%s/dir", pebblePort),
		testTrustedRoots: devTrustedRoots,
	})
	as.NoError(err)

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
}
