package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type handlerVNode struct {
	chord.VNode
	prefixList func(context.Context, []byte) ([][]byte, error)
}

func (n handlerVNode) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	return n.prefixList(ctx, prefix)
}

type handlerTransport struct {
	transport.Transport
	dialStream func(context.Context, *protocol.Node, protocol.Stream_Type) (net.Conn, error)
}

func (tr handlerTransport) DialStream(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	return tr.dialStream(ctx, peer, kind)
}

type handlerQueryService func(context.Context, *protocol.ListTunnelsRequest) (*protocol.ListTunnelsResponse, error)

func (f handlerQueryService) ListTunnels(ctx context.Context, req *protocol.ListTunnelsRequest) (*protocol.ListTunnelsResponse, error) {
	return f(ctx, req)
}

func handlerRouter(node chord.VNode, tr transport.Transport) http.Handler {
	serv := &Server{Config: Config{Chord: node, TunnelTransport: tr}}
	router := chi.NewRouter()
	router.Mount("/clients", TunnelServerHandler(serv))
	return router
}

func handlerClientTransport(t *testing.T, service handlerQueryService, onDial func(context.Context)) transport.Transport {
	t.Helper()
	client := httptest.NewServer(protocol.NewClientQueryServiceServer(service))
	t.Cleanup(client.Close)
	return handlerTransport{dialStream: func(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
		if peer.GetId() != 111111 || peer.GetAddress() != "fake-address" || !peer.GetRendezvous() || kind != protocol.Stream_RPC {
			t.Errorf("unexpected client RPC destination: %v, stream %v", peer, kind)
		}
		if onDial != nil {
			onDial(ctx)
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", client.Listener.Addr().String())
	}}
}

func handlerTunnelInfo(t *testing.T, w *httptest.ResponseRecorder) tunnelsInfo {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var info tunnelsInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &info))
	require.NotNil(t, info.Tunnels, "empty tunnel results must be arrays")
	return info
}

func handlerTunnelRows(t *testing.T, info tunnelsInfo) map[string]clientTunnel {
	t.Helper()
	rows := make(map[string]clientTunnel, len(info.Tunnels))
	for i, tunnel := range info.Tunnels {
		if i > 0 {
			require.Less(t, info.Tunnels[i-1].Hostname, tunnel.Hostname, "tunnels must be sorted and unique")
		}
		rows[tunnel.Hostname] = tunnel
	}
	return rows
}

func TestHandlerListConnectedClients(t *testing.T) {
	node := new(mocks.VNode)
	clientT := new(mocks.Transport)
	clientT.On("ListConnected").Return([]transport.ConnectedPeer{
		{
			Identity: &protocol.Node{Id: 222222, Address: "second-client"},
			Addr:     &net.UDPAddr{IP: net.ParseIP("192.0.2.2"), Port: 4200},
			Version:  "v2.0.0",
		},
		{
			Identity: &protocol.Node{Id: 111111, Address: "fake-address"},
			Addr:     &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 4200},
			Version:  "v1.0.0",
		},
	}).Once()
	clientT.On("Identity").Return(&protocol.Node{Address: "local-node"}).Once()

	w := httptest.NewRecorder()
	handlerRouter(node, clientT).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/clients", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	var observedAt string
	require.NoError(t, json.Unmarshal(body["observedAt"], &observedAt))
	_, err := time.Parse(time.RFC3339, observedAt)
	require.NoError(t, err)
	delete(body, "observedAt")
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"node": "local-node",
		"clients": [
			{"identity":"111111/fake-address","address":"192.0.2.1:4200","version":"v1.0.0","url":"/_internal/tun/111111/fake-address"},
			{"identity":"222222/second-client","address":"192.0.2.2:4200","version":"v2.0.0","url":"/_internal/tun/222222/second-client"}
		]
	}`, string(payload))
	require.Empty(t, node.Calls, "the connected-client list must use only the local transport snapshot")
	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
}

func TestHandlerEmptyConnectedClientsIsArray(t *testing.T) {
	clientT := new(mocks.Transport)
	clientT.On("ListConnected").Return([]transport.ConnectedPeer(nil)).Once()
	clientT.On("Identity").Return(&protocol.Node{Address: "local-node"}).Once()
	w := httptest.NewRecorder()
	handlerRouter(new(mocks.VNode), clientT).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/clients", nil))
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.JSONEq(t, `[]`, string(body["clients"]))
	clientT.AssertExpectations(t)
}

func TestHandlerClientIdentityURLRoundTrip(t *testing.T) {
	for _, address := range []string{`client/<script>alert("address")</script>?x=1&y=2`, "client%2Fescaped"} {
		t.Run(address, func(t *testing.T) {
			clientT := new(mocks.Transport)
			clientT.On("ListConnected").Return([]transport.ConnectedPeer{{
				Identity: &protocol.Node{Id: 111111, Address: address},
				Addr:     &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 4200},
				Version:  `<script>alert("version")</script>`,
			}}).Once()
			clientT.On("Identity").Return(&protocol.Node{Address: `<script>alert("node")</script>`}).Once()
			w := httptest.NewRecorder()
			handlerRouter(new(mocks.VNode), clientT).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/clients", nil))
			var list connectedInfo
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
			require.Len(t, list.Clients, 1)
			require.Equal(t, `<script>alert("node")</script>`, list.Node)
			require.Equal(t, `<script>alert("version")</script>`, list.Clients[0].Version)
			require.Equal(t, "111111/"+address, list.Clients[0].Identity)
			require.Equal(t, "/_internal/tun/111111/"+url.PathEscape(address), list.Clients[0].URL)
			require.NotContains(t, w.Body.String(), "<script>")

			// Follow the listed browser URL against the corresponding API route.
			clientT.On("DialStream", mock.Anything, mock.MatchedBy(func(peer *protocol.Node) bool {
				return peer.GetId() == 111111 && peer.GetAddress() == address && peer.GetRendezvous()
			}), protocol.Stream_RPC).Return(nil, errors.New("client unavailable")).Once()
			node := handlerVNode{prefixList: func(_ context.Context, prefix []byte) ([][]byte, error) {
				expected := tun.ClientHostnamesPrefix(&protocol.ClientToken{Token: []byte(address)})
				if string(prefix) != expected {
					t.Errorf("registration prefix = %q, want %q", prefix, expected)
				}
				return nil, nil
			}}
			w = httptest.NewRecorder()
			path := strings.Replace(list.Clients[0].URL, "/_internal/tun", "/clients", 1)
			handlerRouter(node, clientT).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			info := handlerTunnelInfo(t, w)
			require.Equal(t, address, info.Address)
			require.Equal(t, "111111/"+address, info.Identity)
			clientT.AssertExpectations(t)
		})
	}
}

func TestHandlerListClientTunnels(t *testing.T) {
	for _, tc := range []struct {
		name             string
		configurationErr error
		registrationErr  error
		want             map[string][2]string
	}{
		{
			name: "union of configured and registered hostnames",
			want: map[string][2]string{
				"configured-only": {"Yes", "No"},
				"registered-only": {"No", "Yes"},
				"shared":          {"Yes", "Yes"},
			},
		},
		{
			name:             "client failure preserves registrations",
			configurationErr: errors.New("client unavailable"),
			want: map[string][2]string{
				"registered-only": {"Unknown", "Yes"},
				"shared":          {"Unknown", "Yes"},
			},
		},
		{
			name:            "registration failure preserves client configuration",
			registrationErr: errors.New("ring unavailable"),
			want: map[string][2]string{
				"configured-only": {"Yes", "Unknown"},
				"shared":          {"Yes", "Unknown"},
			},
		},
		{
			name:             "both sources unavailable",
			configurationErr: errors.New("client unavailable"),
			registrationErr:  errors.New("ring unavailable"),
			want:             map[string][2]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientT := handlerClientTransport(t, func(context.Context, *protocol.ListTunnelsRequest) (*protocol.ListTunnelsResponse, error) {
				return &protocol.ListTunnelsResponse{Tunnels: []*protocol.ClientTunnel{
					{Hostname: "shared", Target: "http://shared-target"},
					{Hostname: "configured-only", Target: "http://configured-target"},
				}}, tc.configurationErr
			}, nil)
			node := handlerVNode{prefixList: func(_ context.Context, prefix []byte) ([][]byte, error) {
				expected := tun.ClientHostnamesPrefix(&protocol.ClientToken{Token: []byte("fake-address")})
				if string(prefix) != expected {
					t.Errorf("registration prefix = %q, want %q", prefix, expected)
				}
				return [][]byte{[]byte("shared"), []byte("registered-only"), []byte("shared")}, tc.registrationErr
			}}

			w := httptest.NewRecorder()
			handlerRouter(node, clientT).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/clients/111111/fake-address", nil))

			info := handlerTunnelInfo(t, w)
			require.Equal(t, "111111/fake-address", info.Identity)
			require.Equal(t, "fake-address", info.Address)
			rows := handlerTunnelRows(t, info)
			require.Len(t, rows, len(tc.want))
			for hostname, statuses := range tc.want {
				require.Contains(t, rows, hostname)
				require.Equal(t, statuses[0], rows[hostname].Configured, hostname)
				require.Equal(t, statuses[1], rows[hostname].Registered, hostname)
			}
			if tc.configurationErr != nil {
				require.Contains(t, info.ConfigurationError, tc.configurationErr.Error())
				require.NotContains(t, w.Body.String(), "http://configured-target")
			} else {
				require.Empty(t, info.ConfigurationError)
				require.Equal(t, "http://configured-target", rows["configured-only"].Target)
			}
			if tc.registrationErr != nil {
				require.Equal(t, tc.registrationErr.Error(), info.RegistrationError)
			} else {
				require.Empty(t, info.RegistrationError)
			}
		})
	}
}

func TestHandlerClientTunnelsLookupsShareDeadlineAndRunConcurrently(t *testing.T) {
	configurationStarted := make(chan struct{})
	registrationStarted := make(chan struct{})
	dialContexts := make(chan context.Context, 1)
	registrationContexts := make(chan context.Context, 1)
	clientT := handlerClientTransport(t, func(ctx context.Context, _ *protocol.ListTunnelsRequest) (*protocol.ListTunnelsResponse, error) {
		close(configurationStarted)
		select {
		case <-registrationStarted:
			return &protocol.ListTunnelsResponse{Tunnels: []*protocol.ClientTunnel{{Hostname: "shared", Target: "http://target"}}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, func(ctx context.Context) { dialContexts <- ctx })
	node := handlerVNode{prefixList: func(ctx context.Context, _ []byte) ([][]byte, error) {
		registrationContexts <- ctx
		close(registrationStarted)
		select {
		case <-configurationStarted:
			return [][]byte{[]byte("shared")}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}

	start := time.Now()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/clients/111111/fake-address", nil).WithContext(t.Context())
	handlerRouter(node, clientT).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	rows := handlerTunnelRows(t, handlerTunnelInfo(t, w))
	require.Equal(t, "Yes", rows["shared"].Configured)
	require.Equal(t, "Yes", rows["shared"].Registered)
	dialDeadline, ok := (<-dialContexts).Deadline()
	require.True(t, ok, "the tunnel dial needs a bounded request context")
	registrationDeadline, ok := (<-registrationContexts).Deadline()
	require.True(t, ok)
	require.Equal(t, registrationDeadline, dialDeadline)
	require.WithinDuration(t, start.Add(lookupTimeout), registrationDeadline, time.Second)
}

func TestHandlerClientTunnelsCancellation(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "stalled registration"
		if completed {
			name = "completed registration"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				release := make(chan struct{})
				defer close(release)
				var dialContext, registrationContext context.Context
				dialStopped := make(chan struct{})
				clientT := handlerTransport{dialStream: func(ctx context.Context, _ *protocol.Node, _ protocol.Stream_Type) (net.Conn, error) {
					dialContext = ctx
					defer close(dialStopped)
					<-ctx.Done()
					return nil, ctx.Err()
				}}
				node := handlerVNode{prefixList: func(ctx context.Context, _ []byte) ([][]byte, error) {
					registrationContext = ctx
					if completed {
						return [][]byte{[]byte("registered-only")}, nil
					}
					<-release // Simulate storage that does not honor cancellation.
					return nil, ctx.Err()
				}}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				w := httptest.NewRecorder()
				done := make(chan struct{})
				go func() {
					defer close(done)
					req := httptest.NewRequest(http.MethodGet, "/clients/111111/fake-address", nil).WithContext(ctx)
					handlerRouter(node, clientT).ServeHTTP(w, req)
				}()
				synctest.Wait()
				require.NotNil(t, dialContext)
				require.NotNil(t, registrationContext)
				cancel()
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("handler did not finish after cancellation")
				}
				select {
				case <-dialStopped:
				default:
					t.Fatal("client dial did not stop after cancellation")
				}
				require.ErrorIs(t, dialContext.Err(), context.Canceled)
				require.ErrorIs(t, registrationContext.Err(), context.Canceled)
				info := handlerTunnelInfo(t, w)
				require.Contains(t, info.ConfigurationError, context.Canceled.Error())
				if completed {
					require.Empty(t, info.RegistrationError)
					require.Equal(t, []clientTunnel{{Hostname: "registered-only", Configured: "Unknown", Registered: "Yes"}}, info.Tunnels)
				} else {
					require.Equal(t, context.Canceled.Error(), info.RegistrationError)
					require.Empty(t, info.Tunnels)
				}
			})
		})
	}
}
