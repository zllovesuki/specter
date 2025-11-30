package acme

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/stretchr/testify/require"
)

const (
	pebbleImage = "ghcr.io/letsencrypt/pebble:latest"
	acmePort    = "14000/tcp"
	mgmtPort    = "15000/tcp"
)

// pebbleEnv holds the configuration for connecting to a Pebble ACME server
type pebbleEnv struct {
	ACMEURL      string
	TrustedRoots *x509.CertPool
	container    testcontainers.Container
}

// Stop terminates the Pebble container
func (p *pebbleEnv) Stop(ctx context.Context) error {
	if p.container != nil {
		return testcontainers.TerminateContainer(p.container)
	}
	return nil
}

// StartPebble starts a Pebble ACME test server in a container.
// It requires the following files to exist (run `make certs` first):
//   - dev/pebble/config.json
//   - dev/pebble/certs/cert.pem (Pebble CA)
//   - certs/pebble.pem (Pebble server cert)
//   - certs/pebble.key (Pebble server key)
func StartPebble(t *testing.T) *pebbleEnv {
	t.Helper()
	as := require.New(t)
	ctx := t.Context()

	// Resolve paths relative to this test file
	devPebblePath := path.Join(basepath, "..", "dev", "pebble")
	certsPath := path.Join(basepath, "..", "certs")

	// Read Pebble CA for trusted roots
	pebbleCaPath := path.Join(devPebblePath, "certs", "cert.pem")
	pebbleCa, err := os.ReadFile(pebbleCaPath)
	as.NoError(err, "failed to read Pebble CA; run 'make certs' first")

	trustedRoots := x509.NewCertPool()
	ok := trustedRoots.AppendCertsFromPEM(pebbleCa)
	as.True(ok, "failed to parse Pebble CA certificate")

	// Read files to copy into the container
	configPath := path.Join(devPebblePath, "config.json")
	configData, err := os.ReadFile(configPath)
	as.NoError(err, "failed to read Pebble config.json")

	pebblePemPath := path.Join(certsPath, "pebble.pem")
	pebblePem, err := os.ReadFile(pebblePemPath)
	as.NoError(err, "failed to read certs/pebble.pem; run 'make certs' first")

	pebbleKeyPath := path.Join(certsPath, "pebble.key")
	pebbleKey, err := os.ReadFile(pebbleKeyPath)
	as.NoError(err, "failed to read certs/pebble.key; run 'make certs' first")

	// Create a TLS config that trusts the Pebble CA for the wait strategy
	tlsConfig := &tls.Config{
		RootCAs: trustedRoots,
	}

	// Start Pebble container
	ctr, err := testcontainers.Run(ctx, pebbleImage,
		testcontainers.WithFiles(
			testcontainers.ContainerFile{
				Reader:            bytes.NewReader(configData),
				ContainerFilePath: "/pebble/config.json",
				FileMode:          0o644,
			},
			testcontainers.ContainerFile{
				Reader:            bytes.NewReader(pebblePem),
				ContainerFilePath: "/certs/pebble.pem",
				FileMode:          0o644,
			},
			testcontainers.ContainerFile{
				Reader:            bytes.NewReader(pebbleKey),
				ContainerFilePath: "/certs/pebble.key",
				FileMode:          0o600,
			},
		),
		testcontainers.WithExposedPorts(acmePort, mgmtPort),
		testcontainers.WithEnv(map[string]string{
			"PEBBLE_VA_NOSLEEP":      "1",
			"PEBBLE_VA_ALWAYS_VALID": "1",
		}),
		testcontainers.WithCmd("-config", "/pebble/config.json"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/dir").
				WithTLS(true, tlsConfig).
				WithPort(acmePort).
				WithStartupTimeout(30*time.Second),
		),
	)
	as.NoError(err, "failed to start Pebble container")

	// Get the mapped port
	host, err := ctr.Host(ctx)
	as.NoError(err)

	mappedPort, err := ctr.MappedPort(ctx, acmePort)
	as.NoError(err)

	acmeURL := fmt.Sprintf("https://%s:%s/dir", host, mappedPort.Port())
	t.Logf("Pebble ACME server started at %s", acmeURL)

	return &pebbleEnv{
		ACMEURL:      acmeURL,
		TrustedRoots: trustedRoots,
		container:    ctr,
	}
}
