package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/pow"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap/zaptest"
)

func TestAcmeInstruction(t *testing.T) {
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
	acmeName := "acme.example.com"
	acmeContent := "12345678"

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
				Hostname: testHostname,
				Target:   "tcp://127.0.0.1:2345",
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		s.On("AcmeInstruction", mock.Anything, mock.MatchedBy(func(req *protocol.InstructionRequest) bool {
			match := req.GetHostname() == hostname
			if !match {
				return false
			}
			d, err := pow.VerifySolution(req.GetProof(), pow.Parameters{
				Difficulty: acme.HashcashDifficulty,
				Expires:    acme.HashcashExpires,
				GetSubject: func(pubKey ed25519.PublicKey) string {
					return req.GetHostname()
				},
			})
			if err != nil {
				return false
			}
			return bytes.Equal(d.PubKey, privKey.Public().(ed25519.PublicKey))
		})).Return(&protocol.InstructionResponse{
			Name:    acmeName,
			Content: acmeContent,
		}, nil)

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

	resp, err := client.GetAcmeInstruction(ctx, hostname)
	as.NoError(err)
	as.NotNil(resp)
	as.EqualValues(acmeName, resp.GetName())
	as.EqualValues(acmeContent, resp.GetContent())

	c := &http.Client{
		Timeout: time.Second * 5,
	}
	httpReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/acme/%s", sListener.Addr().String(), hostname), nil)
	as.NoError(err)
	httpResp, err := c.Do(httpReq)
	as.NoError(err)
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	as.NoError(err)
	as.Contains(string(body), resp.GetContent())
}

func TestAcmeValidation(t *testing.T) {
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
				Hostname: testHostname,
				Target:   "tcp://127.0.0.1:2345",
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		s.On("AcmeValidate", mock.Anything, mock.MatchedBy(func(req *protocol.ValidateRequest) bool {
			match := req.GetHostname() == hostname
			if !match {
				return false
			}
			d, err := pow.VerifySolution(req.GetProof(), pow.Parameters{
				Difficulty: acme.HashcashDifficulty,
				Expires:    acme.HashcashExpires,
				GetSubject: func(pubKey ed25519.PublicKey) string {
					return req.GetHostname()
				},
			})
			if err != nil {
				return false
			}
			return bytes.Equal(d.PubKey, privKey.Public().(ed25519.PublicKey))
		})).Return(&protocol.ValidateResponse{
			Apex: testApex,
		}, nil)

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

	resp, err := client.RequestAcmeValidation(ctx, hostname)
	as.NoError(err)
	as.NotNil(resp)
	as.EqualValues(testApex, resp.GetApex())

	c := &http.Client{
		Timeout: time.Second * 5,
	}
	httpReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/validate/%s", sListener.Addr().String(), hostname), nil)
	as.NoError(err)
	httpResp, err := c.Do(httpReq)
	as.NoError(err)
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	as.NoError(err)
	as.Contains(string(body), resp.GetApex())
}
