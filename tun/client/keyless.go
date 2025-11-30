package client

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/Yiling-J/theine-go"
	"go.uber.org/zap"
)

const (
	cacheTotalCost = 1 << 21 // 2MiB
	positiveTTL    = time.Second * 60
	failedTTL      = time.Second * 10
)

type keylessCertificateResult struct {
	err  error
	cert *tls.Certificate
}

type keylessSigner struct {
	cli       *Client
	publicKey crypto.PublicKey
	hostname  string
}

func (k *keylessSigner) Public() crypto.PublicKey {
	return k.publicKey
}

func (k *keylessSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	start := time.Now()
	defer func() {
		k.cli.Logger.Info("Keyless signing request",
			zap.String("hostname", k.hostname),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err),
		)
	}()

	proof, err := k.cli.obtainAcmeProof(k.hostname)
	if err != nil {
		return nil, err
	}

	var hashAlg protocol.KeylessSignRequest_HashAlgorithm
	switch opts.HashFunc() {
	case crypto.SHA256:
		hashAlg = protocol.KeylessSignRequest_SHA256
	case crypto.SHA384:
		hashAlg = protocol.KeylessSignRequest_SHA384
	case crypto.SHA512:
		hashAlg = protocol.KeylessSignRequest_SHA512
	default:
		hashAlg = protocol.KeylessSignRequest_UNKNOWN
	}

	req := &protocol.KeylessSignRequest{
		Proof:    proof,
		Hostname: k.hostname,
		Digest:   digest,
		Algo:     hashAlg,
	}

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	resp, err := retryRPC(k.cli, ctx, func(node *protocol.Node) (*protocol.KeylessSignResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return k.cli.tunnelClient.Sign(ctx, req)
	})
	if err != nil {
		return nil, err
	}

	return resp.GetSignature(), nil
}

var _ crypto.Signer = (*keylessSigner)(nil)

func (c *Client) keylesCertificateCacheLoader(ctx context.Context, hostname string) (ret theine.Loaded[keylessCertificateResult], loadErr error) {
	start := time.Now()
	defer func() {
		c.Logger.Debug("Keyless certificate loader invoked",
			zap.String("hostname", hostname),
			zap.Duration("duration", time.Since(start)),
			zap.Bool("err", ret.Value.err != nil),
			zap.Int64("cost", ret.Cost),
			zap.Duration("ttl", ret.TTL),
		)
	}()

	proof, err := c.obtainAcmeProof(hostname)
	if err != nil {
		return ret, err
	}

	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	cert, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.KeylessGetCertificateResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.GetCertificate(ctx, &protocol.KeylessGetCertificateRequest{
			Proof:    proof,
			Hostname: hostname,
		})
	})
	if err != nil {
		ret.Value.err = err
		ret.TTL = failedTTL
		return
	}

	if len(cert.GetCertificates()) == 0 {
		ret.Value.err = errors.New("no certificate returned by server")
		ret.TTL = failedTTL
		return
	}

	leaf, err := x509.ParseCertificate(cert.GetCertificates()[0])
	if err != nil {
		ret.Value.err = err
		ret.TTL = failedTTL
		return
	}

	pubKey, ok := leaf.PublicKey.(crypto.PublicKey)
	if !ok {
		ret.Value.err = errors.New("leaf public key is not a crypto.PublicKey")
		ret.TTL = failedTTL
		return
	}

	signer := &keylessSigner{
		cli:       c,
		hostname:  hostname,
		publicKey: pubKey,
	}

	keylessCert := &tls.Certificate{
		Certificate: cert.GetCertificates(),
		PrivateKey:  signer,
		Leaf:        leaf,
	}

	ret.Value.cert = keylessCert
	ret.TTL = positiveTTL
	// Cost is only an approximation
	ret.Cost = int64(len(cert.Certificates[0]) + len(leaf.Raw))
	return
}

func (c *Client) getCertificate(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	var (
		ctx      = context.Background()
		hostname = chi.ServerName
	)
	if chi.Context() != nil {
		ctx = chi.Context()
	}

	ret, err := c.keylessCertificateCache.Get(ctx, hostname)
	if ret.err != nil {
		c.Logger.Error("error getting keyless certificate", zap.Error(ret.err))
		return nil, ret.err
	}
	if err != nil {
		c.Logger.Error("error getting keyless certificate", zap.Error(err))
		return nil, err
	}
	return ret.cert, nil
}
