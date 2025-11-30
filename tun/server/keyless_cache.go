package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"go.miragespace.co/specter/spec/rpc"

	"github.com/Yiling-J/theine-go"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

type keylessCertResult struct {
	err  error
	cert *tls.Certificate
}

const (
	keylessCacheBytes  = 1 << 24 // 16MiB
	keylessPositiveTTL = 5 * time.Minute
	keylessFailedTTL   = 10 * time.Second
	keylessExpirySkew  = time.Minute
)

func (s *Server) initKeylessCache() {
	cache, err := theine.NewBuilder[string, keylessCertResult](keylessCacheBytes).
		BuildWithLoader(s.keylessCertLoader)
	if err != nil {
		panic("BUG: " + err.Error())
	}

	s.keylessCache = cache
}

func computeKeylessTTL(cert *tls.Certificate, now time.Time) time.Duration {
	if cert == nil {
		return keylessPositiveTTL
	}

	leaf := cert.Leaf
	if leaf == nil && len(cert.Certificate) > 0 {
		if parsed, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			leaf = parsed
		}
	}
	if leaf == nil {
		return keylessPositiveTTL
	}

	expiry := leaf.NotAfter.Add(-keylessExpirySkew)
	remaining := expiry.Sub(now)
	if remaining <= 0 {
		// certificate is effectively expired (or about to), use a very short ttl
		return time.Second
	}

	if remaining < keylessPositiveTTL {
		return remaining
	}

	return keylessPositiveTTL
}

func (s *Server) keylessCertLoader(ctx context.Context, hostname string) (ret theine.Loaded[keylessCertResult], loadErr error) {
	start := time.Now()
	defer func() {
		if s.Logger != nil {
			s.Logger.Debug("Keyless certificate loader invoked",
				zap.String("hostname", hostname),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("err", ret.Value.err != nil),
				zap.Int64("cost", ret.Cost),
				zap.Duration("ttl", ret.TTL),
			)
		}
	}()

	delegation := rpc.GetDelegation(ctx)
	if delegation == nil {
		ret.Value.err = twirp.Internal.Error("delegation missing in context")
		ret.TTL = keylessFailedTTL
		return
	}

	chi := &tls.ClientHelloInfo{
		ServerName: hostname,
		Conn:       delegation,
	}

	cert, err := s.CertProvider.GetCertificateWithContext(ctx, chi)
	if err != nil {
		ret.Value.err = err
		ret.TTL = keylessFailedTTL
		return
	}
	if cert == nil || len(cert.Certificate) == 0 {
		ret.Value.err = errors.New("no certificate returned from cert provider")
		ret.TTL = keylessFailedTTL
		return
	}

	ret.Value.cert = cert

	now := time.Now()
	ret.TTL = computeKeylessTTL(cert, now)
	if ret.TTL <= 0 {
		ret.TTL = keylessFailedTTL
	}

	if len(cert.Certificate) > 0 {
		ret.Cost = int64(len(cert.Certificate[0]))
		if cert.Leaf != nil {
			ret.Cost += int64(len(cert.Leaf.Raw))
		}
	} else {
		ret.Cost = 1
	}

	return
}
