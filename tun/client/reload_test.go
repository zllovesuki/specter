package client

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap/zaptest"
)

func TestReloadOnSignal(t *testing.T) {
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

	der, cert, key := makeCertificate(as, logger, cl, token, nil)
	cfg := &Config{
		path:        file.Name(),
		router:      skipmap.NewString[route](),
		Apex:        testApex,
		Certificate: cert,
		PrivKey:     key,
		Tunnels: []Tunnel{
			{
				Target: "tcp://127.0.0.1:1234",
			},
		},
	}
	as.NoError(cfg.validate())

	reload := make(chan os.Signal, 1)

	m := func(s *mocks.TunnelService, t1 *mocks.MemoryTransport, publishCall *mock.Call) {
		defaultNoHostnames(s)
		transportHelper(t1, der)
	}

	client, _, assertion := setupClient(t, as, ctx, logger, nil, cfg, reload, m, false, 2)
	defer assertion()
	defer client.Close()

	client.Start(ctx)

	// send reload signal
	reload <- syscall.SIGHUP

	time.Sleep(time.Second)
}
