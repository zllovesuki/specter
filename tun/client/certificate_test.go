package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap/zaptest"
)

func TestRegister_PerformsRenewalWhenNearExpiry(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token := &protocol.ClientToken{
		Token: []byte("test"),
	}
	cl := &protocol.Node{
		Id: chord.Random(),
	}

	// Create a certificate that expires in 1 hour (within renewal window)
	oldDer, oldCert, key := makeCertificateWithExpiry(as, logger, cl, token, nil, time.Hour)

	// Parse the key so we can create the renewed cert with the same key
	privKey, err := pki.UnmarshalPrivateKey([]byte(key))
	as.NoError(err)

	// Create a renewed certificate (fresh, 180 days)
	newDer, newCert, _ := makeCertificate(as, logger, cl, token, privKey)

	cfg := &Config{
		path:        file.Name(),
		router:      skipmap.NewString[route](),
		Apex:        testApex,
		Certificate: oldCert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:2345",
			},
		},
	}
	as.NoError(cfg.validate())

	pkiClient := new(mocks.PKIClient)
	pkiClient.On("RenewCertificate", mock.Anything, mock.Anything).Return(&protocol.CertificateResponse{
		CertDer: newDer,
		CertPem: []byte(newCert),
	}, nil).Once()

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		// First call: OLD certificate (during NewClient initialization)
		// Second call: NEW certificate (after renewal in Register)
		t1.On("WithClientCertificate", mock.Anything).Run(func(args mock.Arguments) {
			cert := args.Get(0).(tls.Certificate)
			parsed, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				panic(err)
			}
			t1.WithCertificate(parsed)
		}).Return(nil).Twice()
		_ = oldDer // used implicitly via oldCert
	}

	client, _, assertion := setupClient(t, as, ctx, logger, pkiClient, cfg, nil, m, true, 1)
	defer assertion()
	defer client.Close()

	// Verify that the certificate was renewed (config should have the new cert)
	currCfg := client.GetCurrentConfig()
	as.Equal(newCert, currCfg.Certificate)
}

func TestShouldRenewCertificate(t *testing.T) {
	as := require.New(t)

	// Test certificate within renewal window (expires in 1 hour)
	nearExpiry := &x509.Certificate{
		NotAfter: time.Now().Add(time.Hour),
	}
	as.True(shouldRenewCertificate(nearExpiry))

	// Test certificate outside renewal window (expires in 60 days)
	farFromExpiry := &x509.Certificate{
		NotAfter: time.Now().Add(60 * 24 * time.Hour),
	}
	as.False(shouldRenewCertificate(farFromExpiry))

	// Test certificate exactly at renewal window boundary (30 days)
	atBoundary := &x509.Certificate{
		NotAfter: time.Now().Add(30 * 24 * time.Hour),
	}
	as.True(shouldRenewCertificate(atBoundary))

	// Test expired certificate (should still return true - needs renewal)
	expired := &x509.Certificate{
		NotAfter: time.Now().Add(-time.Hour),
	}
	as.True(shouldRenewCertificate(expired))
}

func TestCertificateMaintainer_RenewsInBackground(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token := &protocol.ClientToken{
		Token: []byte("test"),
	}
	cl := &protocol.Node{
		Id: chord.Random(),
	}

	// Create a certificate that expires in 1 hour (within renewal window)
	oldDer, oldCert, key := makeCertificateWithExpiry(as, logger, cl, token, nil, time.Hour)

	// Parse the key so we can create the renewed cert with the same key
	privKey, err := pki.UnmarshalPrivateKey([]byte(key))
	as.NoError(err)

	// Create a renewed certificate (fresh, 180 days)
	newDer, newCert, _ := makeCertificate(as, logger, cl, token, privKey)

	cfg := &Config{
		path:        file.Name(),
		router:      skipmap.NewString[route](),
		Apex:        testApex,
		Certificate: oldCert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:2345",
			},
		},
	}
	as.NoError(cfg.validate())

	// Use a channel to signal when RenewCertificate is called
	renewCalled := make(chan struct{}, 1)

	pkiClient := new(mocks.PKIClient)
	pkiClient.On("RenewCertificate", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		select {
		case renewCalled <- struct{}{}:
		default:
		}
	}).Return(&protocol.CertificateResponse{
		CertDer: newDer,
		CertPem: []byte(newCert),
	}, nil).Maybe() // May be called multiple times due to both Register and background maintainer

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		// Certificate may be updated multiple times
		t1.On("WithClientCertificate", mock.Anything).Run(func(args mock.Arguments) {
			cert := args.Get(0).(tls.Certificate)
			parsed, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				panic(err)
			}
			t1.WithCertificate(parsed)
		}).Return(nil).Maybe()
		_ = oldDer // used implicitly via oldCert
	}

	client, _, assertion := setupClient(t, as, ctx, logger, pkiClient, cfg, nil, m, true, 1)
	defer assertion()
	defer client.Close()

	// Start the client (this starts certificateMaintainer goroutine)
	client.Start(ctx)

	// Wait for background renewal to be triggered (certCheckInterval is 100ms in tests)
	// The renewal might have already happened in Register, so we just verify it was called
	select {
	case <-renewCalled:
		// RenewCertificate was called (either in Register or by background maintainer)
	case <-time.After(time.Second):
		t.Fatal("Expected RenewCertificate to be called within timeout")
	}

	// Verify that the certificate was renewed (config should have the new cert)
	currCfg := client.GetCurrentConfig()
	as.Equal(newCert, currCfg.Certificate)

	// Verify PKIClient.RenewCertificate was called at least once
	pkiClient.AssertCalled(t, "RenewCertificate", mock.Anything, mock.Anything)
}
