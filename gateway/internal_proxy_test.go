package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"

	"kon.nect.sh/specter/overlay"
	"kon.nect.sh/specter/spec/cipher"
	mocks "kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util/acceptor"

	"github.com/go-chi/chi/v5"
	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func setupGatewayWithRouter(t *testing.T, as *require.Assertions, logger *zap.Logger, streamRouter *transport.StreamRouter) (udpPort int, tcpPort int, mockS *mocks.TunnelServer, done func()) {
	var q net.PacketConn
	var h2 net.Listener

	q, udpPort = getUDPListener(as)

	h2, tcpPort = getH2Listener(as)

	ss := generateTLSConfig([]string{})
	alpnMux, err := overlay.NewMux(&quic.Transport{Conn: q})
	as.NoError(err)

	h3 := alpnMux.With(cipher.GetGatewayTLSConfig(func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &ss.Certificates[0], nil
	}, nil), append(cipher.H3Protos, tun.ALPN(protocol.Link_TCP))...)

	mockS = new(mocks.TunnelServer)

	ctx, cancel := context.WithCancel(context.Background())

	fakeStats := chi.NewRouter()
	fakeStats.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	conf := GatewayConfig{
		Logger:       logger,
		TunnelServer: mockS,
		HTTPListener: nil,
		H2Listener:   h2,
		H3Listener:   h3,
		RootDomains:  []string{testDomain},
		GatewayPort:  udpPort,
		AdminUser:    os.Getenv("INTERNAL_USER"),
		AdminPass:    os.Getenv("INTERNAL_PASS"),
		Handlers: InternalHandlers{
			Chord: fakeStats,
		},
		Options: Options{
			TransportBufferSize: 1024 * 8,
			ProxyBufferSize:     1024 * 8,
		},
	}
	g := New(conf)
	g.AttachRouter(ctx, streamRouter)
	g.MustStart(ctx)

	go alpnMux.Accept(ctx)

	return udpPort, tcpPort, mockS, func() {
		cancel()
		h2.Close()
		h3.Close()
		alpnMux.Close()
		g.Close()
	}
}

func TestInternalProxy(t *testing.T) {
	os.Setenv("INTERNAL_USER", testUser)
	os.Setenv("INTERNAL_PASS", testPass)
	defer func() {
		os.Setenv("INTERNAL_USER", "")
		os.Setenv("INTERNAL_PASS", "")
	}()

	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	tp := mocks.SelfTransport()
	streamRouter := transport.NewStreamRouter(logger, tp, nil)

	_, tcpPort, mockS, done := setupGatewayWithRouter(t, as, logger, streamRouter)
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go streamRouter.Accept(ctx)

	fakeNode := &protocol.Node{
		Address: "127.0.0.1:4444",
	}

	respString := "proxied"
	fakeServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, respString)
		}),
	}
	acc := acceptor.NewH2Acceptor(nil)
	go fakeServer.Serve(acc)

	c1, c2 := net.Pipe()

	mockS.On("DialInternal", mock.Anything, mock.MatchedBy(func(node *protocol.Node) bool {
		return node.GetAddress() == fakeNode.GetAddress() && node.GetId() == fakeNode.GetId()
	})).Return(c1, nil).Run(func(args mock.Arguments) {
		acc.Handle(c2)
	})

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/_internal/chord/stats", testDomain), nil)
	as.NoError(err)

	req.SetBasicAuth(testUser, testPass)
	req.Header.Set(internalProxyNodeAddress, fakeNode.GetAddress())

	c := getH2Client("", tcpPort)

	resp, err := c.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(b), respString)
	as.NotEmpty(resp.Header.Get("alt-svc"))
	as.Equal("false", resp.Header.Get("http3"))

	mockS.AssertExpectations(t)
}
