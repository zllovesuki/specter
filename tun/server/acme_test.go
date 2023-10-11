package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"

	"go.miragespace.co/specter/spec/acme"
	mocks "go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/pow"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAcmeInstruction(t *testing.T) {
	as := require.New(t)

	logger, node, _, _, serv := getFixture(t, as)
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname := "external.example.com"
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	name, content := acme.GenerateCustomRecord(hostname, testAcmeZone, token.GetToken())

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
	).Return(nil, tun.ErrHostnameNotFound).Once()
	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	privKey := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	tp.WithCertificate(toCertificate(as, logger, cli, token, withExtractPrivKey(privKey)))

	proof, err := pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
	as.NoError(err)

	resp, err := cRPC.AcmeInstruction(rpc.WithNode(ctx, cli), &protocol.InstructionRequest{
		Proof:    proof,
		Hostname: hostname,
	})

	as.NoError(err)
	as.NotNil(resp)

	as.Equal(name, resp.GetName())
	as.Equal(content, resp.GetContent())

	node.AssertExpectations(t)
}

func TestAcmeValidationSuccess(t *testing.T) {
	as := require.New(t)

	resolver := new(mocks.Resolver)

	logger, node, _, _, serv := getFixture(t, as, withResolver(resolver))
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname := "external.example.com"
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	name, content := acme.GenerateCustomRecord(hostname, testAcmeZone, token.GetToken())

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
	).Return(nil, tun.ErrHostnameNotFound).Once()
	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()
	node.On("Put",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
		mock.Anything,
	).Return(nil).Once()
	node.On("PrefixAppend",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(hostname))
		}),
	).Return(nil).Once()

	resolver.On("LookupCNAME", mock.Anything, name).Return(content, nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	privKey := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	tp.WithCertificate(toCertificate(as, logger, cli, token, withExtractPrivKey(privKey)))

	proof, err := pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
	as.NoError(err)

	resp, err := cRPC.AcmeValidate(rpc.WithNode(ctx, cli), &protocol.ValidateRequest{
		Proof:    proof,
		Hostname: hostname,
	})

	as.NoError(err)
	as.NotNil(resp)
	as.Equal(testRootDomain, resp.GetApex())

	node.AssertExpectations(t)
	resolver.AssertExpectations(t)
}

func TestAcmeValidationAlready(t *testing.T) {
	as := require.New(t)

	resolver := new(mocks.Resolver)

	logger, node, _, _, serv := getFixture(t, as, withResolver(resolver))
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname := "external.example.com"
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}
	cli.Address = string(token.GetToken())

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	bundle := &protocol.CustomHostname{
		ClientIdentity: cli,
		ClientToken:    token,
	}

	bundleBuf, err := bundle.MarshalVT()
	as.NoError(err)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
	).Return(bundleBuf, nil).Once()
	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()
	node.On("Put",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
		mock.Anything,
	).Return(nil).Once()
	node.On("PrefixAppend",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientHostnamesPrefix(token)))
		}),
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(hostname))
		}),
	).Return(nil).Once()

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	privKey := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	tp.WithCertificate(toCertificate(as, logger, cli, token, withExtractPrivKey(privKey)))

	proof, err := pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
	as.NoError(err)

	resp, err := cRPC.AcmeValidate(rpc.WithNode(ctx, cli), &protocol.ValidateRequest{
		Proof:    proof,
		Hostname: hostname,
	})

	as.NoError(err)
	as.NotNil(resp)
	as.Equal(testRootDomain, resp.GetApex())

	node.AssertExpectations(t)
	resolver.AssertExpectations(t)
}

func TestAcmeValidationIncorrect(t *testing.T) {
	as := require.New(t)

	resolver := new(mocks.Resolver)

	logger, node, _, _, serv := getFixture(t, as, withResolver(resolver))
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname := "external.example.com"
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	name, _ := acme.GenerateCustomRecord(hostname, testAcmeZone, token.GetToken())

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
	).Return(nil, tun.ErrHostnameNotFound).Once()
	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()

	resolver.On("LookupCNAME", mock.Anything, name).Return("random string", nil)

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	privKey := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	tp.WithCertificate(toCertificate(as, logger, cli, token, withExtractPrivKey(privKey)))

	proof, err := pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
	as.NoError(err)

	_, err = cRPC.AcmeValidate(rpc.WithNode(ctx, cli), &protocol.ValidateRequest{
		Proof:    proof,
		Hostname: hostname,
	})

	as.Error(err)

	node.AssertExpectations(t)
	resolver.AssertExpectations(t)
}

func TestAcmeValidationError(t *testing.T) {
	as := require.New(t)

	resolver := new(mocks.Resolver)

	logger, node, _, _, serv := getFixture(t, as, withResolver(resolver))
	cli, _, _ := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname := "external.example.com"
	token := &protocol.ClientToken{
		Token: mustGenerateToken(),
	}

	clientBuf, err := cli.MarshalVT()
	as.NoError(err)

	name, _ := acme.GenerateCustomRecord(hostname, testAcmeZone, token.GetToken())

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
	).Return(nil, tun.ErrHostnameNotFound).Once()
	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil).Once()

	resolver.On("LookupCNAME", mock.Anything, name).Return("", fmt.Errorf("error"))

	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, nil, tp)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	cRPC := rpc.DynamicTunnelClient(rpc.DisablePooling(ctx), tp)

	privKey := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	tp.WithCertificate(toCertificate(as, logger, cli, token, withExtractPrivKey(privKey)))

	proof, err := pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
	as.NoError(err)

	_, err = cRPC.AcmeValidate(rpc.WithNode(ctx, cli), &protocol.ValidateRequest{
		Proof:    proof,
		Hostname: hostname,
	})

	as.Error(err)

	node.AssertExpectations(t)
	resolver.AssertExpectations(t)
}
