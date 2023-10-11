package client

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap/zaptest"
)

func TestQueryRPC(t *testing.T) {
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

	testTarget := "tcp://127.0.0.1:22"

	der, cert, key := makeCertificate(as, logger, cl, token, nil)
	cfg := &Config{
		path:        file.Name(),
		router:      skipmap.NewString[route](),
		Apex:        testApex,
		Certificate: cert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Hostname: testHostname,
				Target:   testTarget,
			},
		},
	}
	as.NoError(cfg.validate())

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, t2, assertion := setupClient(t, as, ctx, logger, nil, cfg, nil, m, false, 1)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	httpCl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: -1,
			DisableKeepAlives:   true,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				c1, err := t2.DialStream(ctx, cl, protocol.Stream_RPC)
				as.NoError(err)
				return c1, nil
			},
		},
	}

	rpcClient := protocol.NewClientQueryServiceProtobufClient("http://client", httpCl)
	resp, err := rpcClient.ListTunnels(ctx, &protocol.ListTunnelsRequest{})
	as.NoError(err)
	as.Len(resp.GetTunnels(), 1)

	tunnel := resp.GetTunnels()[0]
	as.Equal(testHostname, tunnel.GetHostname())
	as.Equal(testTarget, tunnel.GetTarget())
}
