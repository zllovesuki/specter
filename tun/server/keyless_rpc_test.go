package server

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.miragespace.co/specter/spec/acme"
	mocks "go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/pow"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"
)

func TestKeylessGetCertificate(t *testing.T) {
	as := require.New(t)

	resolver := new(mocks.Resolver)
	certProvider := new(mocks.CertProvider)

	logger, node, _, _, serv := getFixture(t, as, withResolver(resolver))
	cli, _, _ := getIdentities()

	serv.Config.CertProvider = certProvider

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

	certProvider.On("GetCertificateWithContext",
		mock.Anything,
		mock.MatchedBy(func(chi *tls.ClientHelloInfo) bool {
			return chi.ServerName == hostname
		}),
	).Return(&tls.Certificate{
		Certificate: [][]byte{
			[]byte("123"),
		},
	}, nil)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
	).Return(bundleBuf, nil)
	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)
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

	cert, err := cRPC.GetCertificate(rpc.WithNode(ctx, cli), &protocol.KeylessGetCertificateRequest{
		Proof:    proof,
		Hostname: hostname,
	})
	as.NoError(err)
	as.NotNil(cert)
	as.Equal([]byte("123"), cert.GetCertificates()[0])

	node.AssertExpectations(t)
	resolver.AssertExpectations(t)
}

func TestKeylessSign(t *testing.T) {
	as := require.New(t)

	resolver := new(mocks.Resolver)
	certProvider := new(mocks.CertProvider)

	logger, node, _, _, serv := getFixture(t, as, withResolver(resolver))
	cli, _, _ := getIdentities()

	serv.Config.CertProvider = certProvider

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

	_, providerX509Cert, providerPrivateKey := testMakeRSACert(as)
	certProvider.On("GetCertificateWithContext",
		mock.Anything,
		mock.MatchedBy(func(chi *tls.ClientHelloInfo) bool {
			return chi.ServerName == hostname
		}),
	).Return(&tls.Certificate{
		Certificate: [][]byte{providerX509Cert.Raw},
		Leaf:        providerX509Cert,
		PrivateKey:  providerPrivateKey,
	}, nil)

	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.CustomHostnameKey(hostname)))
		}),
	).Return(bundleBuf, nil)
	node.On("Get",
		mock.Anything,
		mock.MatchedBy(func(key []byte) bool {
			return bytes.Equal(key, []byte(tun.ClientTokenKey(token)))
		}),
	).Return(clientBuf, nil)
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

	rawMsg := []byte("hello world")
	digest := sha256.Sum256(rawMsg)
	sig, err := cRPC.Sign(rpc.WithNode(ctx, cli), &protocol.KeylessSignRequest{
		Proof:    proof,
		Hostname: hostname,
		Digest:   digest[:],
		Algo:     protocol.KeylessSignRequest_SHA256,
	})
	as.NoError(err)
	as.NotNil(sig)
	as.NoError(rsa.VerifyPKCS1v15(providerX509Cert.PublicKey.(*rsa.PublicKey), crypto.SHA256, digest[:], sig.GetSignature()))

	node.AssertExpectations(t)
	resolver.AssertExpectations(t)
}

func testMakeRSACert(as *require.Assertions) (derBytes []byte, x509Cert *x509.Certificate, privateKey *rsa.PrivateKey) {
	var err error

	privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	as.NoError(err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"dev"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 365), // Valid for 1 year

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err = x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	as.NoError(err)

	x509Cert, err = x509.ParseCertificate(derBytes)
	as.NoError(err)

	return
}
