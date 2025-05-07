package server

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"

	"go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/twitchtv/twirp"
)

func (s *Server) getCertificate(ctx context.Context, proof *protocol.ProofOfWork, hostname string) (*tls.Certificate, error) {
	token, client, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	normalized, err := acme.Normalize(hostname)
	if err != nil {
		return nil, twirp.InvalidArgumentError("hostname", err.Error())
	}

	found, err := s.checkAcme(ctx, normalized, proof, token, client)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, twirp.PermissionDenied.Error("cannot use provided hostname for keyless tls")
	}

	delegation := rpc.GetDelegation(ctx)
	if delegation == nil {
		return nil, twirp.Internal.Error("delegation missing in context")
	}

	return s.CertProvider.GetCertificateWithContext(ctx, &tls.ClientHelloInfo{
		ServerName: hostname,
		Conn:       delegation,
	})
}

func (s *Server) GetCertificate(ctx context.Context, req *protocol.KeylessGetCertificateRequest) (*protocol.KeylessGetCertificateResponse, error) {
	cert, err := s.getCertificate(ctx, req.GetProof(), req.GetHostname())
	if err != nil {
		return nil, err
	}

	return &protocol.KeylessGetCertificateResponse{
		Certificates: cert.Certificate,
	}, nil
}

func (s *Server) Sign(ctx context.Context, req *protocol.KeylessSignRequest) (*protocol.KeylessSignResponse, error) {
	cert, err := s.getCertificate(ctx, req.GetProof(), req.GetHostname())
	if err != nil {
		return nil, err
	}

	var opts crypto.SignerOpts
	switch req.GetAlgo() {
	case protocol.KeylessSignRequest_SHA256:
		opts = crypto.SHA256
	case protocol.KeylessSignRequest_SHA384:
		opts = crypto.SHA384
	case protocol.KeylessSignRequest_SHA512:
		opts = crypto.SHA512
	default:
		return nil, twirp.InvalidArgumentError("algo", "unsupported hash algorithm")
	}

	signer, ok := cert.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, twirp.InternalError("private key is not a crypto.Signer")
	}

	if len(req.GetDigest()) != opts.HashFunc().Size() {
		return nil, twirp.InvalidArgumentError("digest", "invalid digest length")
	}

	sig, err := signer.Sign(rand.Reader, req.Digest, opts)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	return &protocol.KeylessSignResponse{
		Signature: sig,
	}, nil
}
