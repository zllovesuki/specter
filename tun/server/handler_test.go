package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandlerListConnectedClients(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, _, serv := getFixture(t, as)

	fakeClients := []transport.ConnectedPeer{
		{
			Identity: &protocol.Node{
				Id:      111111,
				Address: "fake-address",
			},
			Addr: &net.UDPAddr{},
		},
	}

	hostnames := []string{"hostname-A", "hostname-B"}
	hostnameBytes := make([][]byte, len(hostnames))
	for i, h := range hostnames {
		hostnameBytes[i] = []byte(h)
	}

	node.On("PrefixList",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(&protocol.ClientToken{
				Token: []byte("fake-address"),
			})))
		}),
	).Return(hostnameBytes, nil)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("AcceptStream").Return(clientChan)
	clientT.On("ListConnected").Return(fakeClients)
	clientT.On("Identity").Return(&protocol.Node{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamRouter := transport.NewStreamRouter(logger, nil, clientT)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	router := chi.NewRouter()

	router.Mount("/clients", TunnelServerHandler(serv))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/clients", nil)

	router.ServeHTTP(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(body), "fake-address")
	as.Contains(string(body), "2")

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestHandlerListClientTunnels(t *testing.T) {
	as := require.New(t)

	logger, node, clientT, _, serv := getFixture(t, as)

	fakeTunnels := &protocol.ListTunnelsResponse{
		Tunnels: []*protocol.ClientTunnel{
			{
				Hostname: "test-hostname",
				Target:   "http://test-target",
			},
		},
	}
	fakeTunnelsBytes, err := fakeTunnels.MarshalVT()
	as.NoError(err)

	hostnames := []string{"test-hostname", "not-used"}
	hostnameBytes := make([][]byte, len(hostnames))
	for i, h := range hostnames {
		hostnameBytes[i] = []byte(h)
	}

	c1, c2 := net.Pipe()

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("AcceptStream").Return(clientChan)
	clientT.On("DialStream", mock.Anything, mock.MatchedBy(func(node *protocol.Node) bool {
		return node.GetId() == 111111 && node.GetAddress() == "fake-address" && node.GetRendezvous()
	}), protocol.Stream_RPC).Return(c1, nil)
	node.On("PrefixList",
		mock.Anything,
		mock.MatchedBy(func(prefix []byte) bool {
			return bytes.Equal(prefix, []byte(tun.ClientHostnamesPrefix(&protocol.ClientToken{
				Token: []byte("fake-address"),
			})))
		}),
	).Return(hostnameBytes, nil)

	go func() {
		_, err := http.ReadRequest(bufio.NewReader(c2))
		as.NoError(err)
		resp := http.Response{
			StatusCode:    http.StatusOK,
			ProtoMajor:    1,
			ProtoMinor:    1,
			ContentLength: int64(len(fakeTunnelsBytes)),
			Body:          io.NopCloser(bytes.NewBuffer(fakeTunnelsBytes)),
		}
		resp.Write(c2)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamRouter := transport.NewStreamRouter(logger, nil, clientT)
	go streamRouter.Accept(ctx)

	serv.AttachRouter(ctx, streamRouter)

	router := chi.NewRouter()

	router.Mount("/clients", TunnelServerHandler(serv))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://test/clients/111111/fake-address", nil)

	router.ServeHTTP(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	as.NoError(err)

	as.Contains(string(body), fakeTunnels.GetTunnels()[0].GetHostname())
	as.Contains(string(body), fakeTunnels.GetTunnels()[0].GetTarget())
	as.Contains(string(body), "not-used")
	as.Contains(string(body), "(unused)")

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}
