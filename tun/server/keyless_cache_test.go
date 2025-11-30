package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap/zaptest"
)

func TestComputeKeylessTTL_UsesBaseWhenNoLeaf(t *testing.T) {
	as := require.New(t)

	now := time.Now()
	cert := &tls.Certificate{}

	ttl := computeKeylessTTL(cert, now)

	as.Equal(keylessPositiveTTL, ttl)
}

func TestComputeKeylessTTL_ClampedByExpiry(t *testing.T) {
	as := require.New(t)

	now := time.Now()
	leaf := &x509.Certificate{NotAfter: now.Add(2 * time.Minute)}
	cert := &tls.Certificate{Leaf: leaf}

	ttl := computeKeylessTTL(cert, now)

	expected := 2*time.Minute - keylessExpirySkew
	as.InDelta(expected.Seconds(), ttl.Seconds(), 0.01)
}

func TestComputeKeylessTTL_ExpiredUsesShortTTL(t *testing.T) {
	as := require.New(t)

	now := time.Now()
	leaf := &x509.Certificate{NotAfter: now.Add(-time.Minute)}
	cert := &tls.Certificate{Leaf: leaf}

	ttl := computeKeylessTTL(cert, now)

	as.Equal(time.Second, ttl)
}

func TestKeylessCertLoaderSuccess(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	provider := new(mocks.CertProvider)
	s := &Server{
		Config: Config{
			Logger:       logger,
			CertProvider: provider,
		},
	}

	hostname := "example.com"
	leaf := &x509.Certificate{NotAfter: time.Now().Add(time.Hour)}
	tlsCert := &tls.Certificate{
		Certificate: [][]byte{{1, 2, 3}},
		Leaf:        leaf,
	}

	delegate := &transport.StreamDelegate{}
	provider.On("GetCertificateWithContext",
		mock.Anything,
		mock.MatchedBy(func(chi *tls.ClientHelloInfo) bool {
			as.Equal(hostname, chi.ServerName)
			as.NotNil(chi.Conn)
			return true
		}),
	).Return(tlsCert, nil)

	ctx := rpc.WithDelegation(context.Background(), delegate)
	ret, err := s.keylessCertLoader(ctx, hostname)

	as.NoError(err)
	as.NoError(ret.Value.err)
	as.Same(tlsCert, ret.Value.cert)
	as.True(ret.TTL > 0)
	as.True(ret.Cost > 0)

	provider.AssertExpectations(t)
}

func TestKeylessCertLoaderProviderError(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	provider := new(mocks.CertProvider)
	s := &Server{
		Config: Config{
			Logger:       logger,
			CertProvider: provider,
		},
	}

	hostname := "example.com"
	delegate := &transport.StreamDelegate{}
	provErr := errors.New("boom")

	provider.On("GetCertificateWithContext", mock.Anything, mock.Anything).
		Return((*tls.Certificate)(nil), provErr)

	ctx := rpc.WithDelegation(context.Background(), delegate)
	ret, err := s.keylessCertLoader(ctx, hostname)

	as.NoError(err)
	as.Equal(provErr, ret.Value.err)
	as.Equal(keylessFailedTTL, ret.TTL)

	provider.AssertExpectations(t)
}

func TestKeylessCertLoaderMissingDelegation(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	provider := new(mocks.CertProvider)
	s := &Server{
		Config: Config{
			Logger:       logger,
			CertProvider: provider,
		},
	}

	hostname := "example.com"

	ret, err := s.keylessCertLoader(context.Background(), hostname)

	as.NoError(err)
	as.NotNil(ret.Value.err)
	twerr, ok := ret.Value.err.(twirp.Error)
	as.True(ok)
	as.Equal(twirp.Internal, twerr.Code())
	as.Equal(keylessFailedTTL, ret.TTL)

	provider.AssertExpectations(t)
}
