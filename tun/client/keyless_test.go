package client

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
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

func TestKeylessSign(t *testing.T) {
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

	hostname := "custom.domain.com"

	der, cert, key := makeCertificate(as, logger, cl, token, nil)
	privKey, err := pki.UnmarshalPrivateKey([]byte(key))
	as.NoError(err)

	cfg := &Config{
		path:        file.Name(),
		router:      skipmap.NewString[route](),
		Apex:        testApex,
		Certificate: cert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:2345",
			},
		},
	}
	as.NoError(cfg.validate())

	_, providerX509Cert, providerPrivateKey := testMakeRSACert(as)
	providerCert := &tls.Certificate{
		Certificate: [][]byte{providerX509Cert.Raw},
		PrivateKey:  providerPrivateKey,
		Leaf:        providerX509Cert,
	}

	rawMsg := []byte("hello world")
	digest := sha256.Sum256(rawMsg)
	algoUsed := protocol.KeylessSignRequest_SHA256
	providerSig, err := providerPrivateKey.Sign(rand.Reader, digest[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
		Hash:       crypto.SHA256,
	})
	as.NoError(err)

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		s.Keyless.On("GetCertificate", mock.Anything, mock.MatchedBy(func(req *protocol.KeylessGetCertificateRequest) bool {
			return powValidateFunc(hostname, privKey)(req.GetProof(), req.GetHostname())
		})).Return(&protocol.KeylessGetCertificateResponse{
			Certificates: providerCert.Certificate,
		}, nil)
		s.Keyless.On("Sign", mock.Anything, mock.MatchedBy(func(req *protocol.KeylessSignRequest) bool {
			powValid := powValidateFunc(hostname, privKey)(req.GetProof(), req.GetHostname())
			if !powValid {
				return false
			}
			return req.GetAlgo() == algoUsed && bytes.Equal(digest[:], req.GetDigest())
		})).Return(&protocol.KeylessSignResponse{
			Signature: providerSig,
		}, nil)

		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	listenCfg := &net.ListenConfig{}
	sListener, err := listenCfg.Listen(ctx, "tcp", "127.0.0.1:0")
	as.NoError(err)
	defer sListener.Close()

	client, _, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, false, 1)
	defer assertion()
	defer client.Close()

	client.ServerListener = sListener

	client.Start(ctx)

	keylessCert, err := client.getCertificate(&tls.ClientHelloInfo{
		ServerName: hostname,
	})
	as.NoError(err)
	as.NotNil(keylessCert)

	keylessSigner, ok := keylessCert.PrivateKey.(crypto.Signer)
	as.True(ok)

	keylessSignature, err := keylessSigner.Sign(rand.Reader, digest[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
		Hash:       crypto.SHA256,
	})
	as.NoError(err)
	as.NotNil(keylessSignature)
	as.EqualValues(providerSig, keylessSignature)
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
