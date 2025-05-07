package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/pow"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

func (s *Server) checkAcme(ctx context.Context, hostname string, proof *protocol.ProofOfWork, token *protocol.ClientToken, client *protocol.Node) (found bool, err error) {
	_, err = pow.VerifySolution(proof, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
	if err != nil {
		return false, err
	}

	if strings.Contains(hostname, s.Acme) || strings.Contains(hostname, s.Apex) {
		return false, twirp.InvalidArgumentError("hostname", "provided hostname is not valid for custom hostname")
	}

	if strings.Count(hostname, ".") < 2 {
		return false, twirp.InvalidArgumentError("hostname", "custom domain is not supported on bare domain")
	}

	bundle, err := tun.FindCustomHostname(ctx, s.Chord, hostname)
	switch err {
	case nil:
		// alreday exist
		if !bytes.Equal(bundle.GetClientToken().GetToken(), token.GetToken()) ||
			bundle.GetClientIdentity().GetId() != client.GetId() ||
			bundle.GetClientIdentity().GetAddress() != client.GetAddress() {
			return false, twirp.InvalidArgumentError("hostname", "provided hostname is not valid for custom hostname")
		}
		return true, nil
	case tun.ErrHostnameNotFound:
		// expected
		return false, nil
	default:
		return false, err
	}
}

func (s *Server) AcmeInstruction(ctx context.Context, req *protocol.InstructionRequest) (*protocol.InstructionResponse, error) {
	token, client, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	hostname, err := acme.Normalize(req.GetHostname())
	if err != nil {
		return nil, twirp.InvalidArgumentError("hostname", err.Error())
	}

	if _, err := s.checkAcme(ctx, hostname, req.GetProof(), token, client); err != nil {
		return nil, err
	}

	name, content := acme.GenerateCustomRecord(hostname, s.Acme, token.GetToken())
	return &protocol.InstructionResponse{
		Name:    name,
		Content: content,
	}, nil
}

func (s *Server) AcmeValidate(ctx context.Context, req *protocol.ValidateRequest) (*protocol.ValidateResponse, error) {
	token, client, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	hostname, err := acme.Normalize(req.GetHostname())
	if err != nil {
		return nil, twirp.InvalidArgumentError("hostname", err.Error())
	}

	found, err := s.checkAcme(ctx, hostname, req.GetProof(), token, client)
	if err != nil {
		return nil, err
	}

	var (
		start time.Time
		cname string
	)

	name, content := acme.GenerateCustomRecord(hostname, s.Acme, token.GetToken())

	lookupCtx, lookupCancel := context.WithTimeout(ctx, lookupTimeout)
	defer lookupCancel()

	if found {
		goto VALIDATED
	}

	start = time.Now()
	cname, err = s.Resolver.LookupCNAME(lookupCtx, name)
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}

	if cname != content {
		return nil, twirp.FailedPrecondition.Errorf("unexpected CNAME content: %s", cname)
	}

	s.Logger.Info("Custom hostname validated", zap.String("hostname", hostname), zap.Duration("took", time.Since(start)), zap.Object("client", client))

VALIDATED:
	if err := tun.SaveCustomHostname(ctx, s.Chord, hostname, &protocol.CustomHostname{
		ClientIdentity: client,
		ClientToken:    token,
	}); err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	prefix := tun.ClientHostnamesPrefix(token)
	err = s.Chord.PrefixAppend(ctx, []byte(prefix), []byte(hostname))
	if err != nil && !errors.Is(err, chord.ErrKVPrefixConflict) {
		return nil, twirp.InternalErrorWith(err)
	}

	return &protocol.ValidateResponse{
		Apex: s.Apex,
	}, nil
}
