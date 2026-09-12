package client

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/tun/client/dialer"
	"go.miragespace.co/specter/util/acceptor"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type lightweightTransport struct {
	*mocks.MemoryTransport
	cert        *x509.Certificate
	attempts    chan *mocks.PhysicalConn
	addresses   chan string
	mu          sync.Mutex
	aliases     map[string]string
	connections map[string]*mocks.PhysicalConn
}

func (t *lightweightTransport) DialStream(ctx context.Context, node *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	address := node.GetAddress()
	t.mu.Lock()
	if canonical, ok := t.aliases[address]; ok {
		node = &protocol.Node{Address: canonical}
	}
	pc := t.connections[node.GetAddress()]
	if pc == nil || pc.Err() != nil {
		pc = mocks.NewPhysicalConn(func(d *transport.StreamDelegate) {
			d.Certificate, d.Identity = t.cert, node
			if d.Kind == protocol.Stream_DIRECT {
				t.Self <- d
			} else {
				t.Other <- d
			}
		})
		if t.connections != nil {
			t.connections[node.GetAddress()] = pc
		}
	}
	t.mu.Unlock()
	if t.attempts != nil {
		t.attempts <- pc
	}
	if t.addresses != nil {
		t.addresses <- address
	}
	return pc.OpenStream(kind)
}

func lightweightUpstream(t *testing.T, pc *mocks.PhysicalConn, hostname string) string {
	t.Helper()
	direct, err := pc.OpenStream(protocol.Stream_DIRECT)
	require.NoError(t, err)
	require.NoError(t, rpc.Send(direct, &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: hostname,
	}))
	tp := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) { return direct, nil }}
	defer tp.CloseIdleConnections()
	hc := &http.Client{
		Transport: tp,
		Timeout:   time.Second,
	}
	resp, err := hc.Get("http://" + hostname + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}

func TestLightweightRun(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "upstream") }))
	defer target.Close()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, _, _ := makeCertificate(require.New(t), logger, &protocol.Node{Id: 1}, &protocol.ClientToken{Token: []byte("test")}, key)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pki := &mocks.PKIClient{}
	pki.On("RequestCertificate", mock.Anything, mock.Anything).Return(&protocol.CertificateResponse{CertDer: der}, nil).Once()
	left, right := mocks.PipeTransport()
	tp := &lightweightTransport{
		MemoryTransport: left,
		cert:            cert,
		attempts:        make(chan *mocks.PhysicalConn, 4),
		addresses:       make(chan string, 4),
	}
	left.On("WithClientCertificate", mock.AnythingOfType("tls.Certificate")).Run(func(args mock.Arguments) { require.Len(t, args.Get(0).(tls.Certificate).Certificate, 1) }).Return(nil).Once()
	service := &mocks.TunnelService{}
	home := &protocol.Node{Address: "home.example.com:443"}
	response := &protocol.OpenSessionResponse{
		Hostname: "temporary",
		Node:     home,
		Apex:     "example.com",
	}
	service.On("OpenEphemeralSession", mock.Anything, mock.Anything).Return(nil, twirp.Unavailable.Error("try again")).Once()
	opened := make(chan context.Context, 1)
	release := make(chan struct{})
	service.On("OpenEphemeralSession", mock.Anything, mock.Anything).Run(func(args mock.Arguments) { opened <- args.Get(0).(context.Context); <-release }).Return(response, nil).Once()
	service.On("OpenEphemeralSession", mock.Anything, mock.Anything).Return(response, nil).Once()
	service.On("OpenEphemeralSession", mock.Anything, mock.Anything).Return(nil, twirp.PermissionDenied.Error("denied")).Once()
	router := transport.NewStreamRouter(logger, nil, right)
	acc := acceptor.NewH2Acceptor(nil)
	defer acc.Close()
	setupRPC(ctx, logger, service, &mocks.KeylessService{}, router, acc)
	go router.Accept(ctx)
	outputR, outputW := io.Pipe()
	defer outputR.Close()
	l, err := NewLightweightClient(LightweightConfig{
		Logger:    logger,
		Transport: tp,
		PKIClient: pki,
		Apex: &dialer.ParsedApex{
			Host: "example.com",
			Port: 443,
		},
		Target: target.URL,
		Output: outputW,
	})
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() { result <- l.Run(ctx); outputW.Close() }()
	first := <-tp.attempts
	require.Equal(t, "example.com:443", <-tp.addresses)
	require.Eventually(t, func() bool { return first.Err() != nil }, 3*time.Second, time.Millisecond)
	second := <-tp.attempts
	require.NotSame(t, first, second)
	require.Equal(t, "example.com:443", <-tp.addresses)
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal("Open not reached")
	}
	require.Equal(t, "upstream", lightweightUpstream(t, second, "temporary"))
	close(release)
	printed := make(chan string, 1)
	go func() { data, _ := io.ReadAll(outputR); printed <- string(data) }()
	require.Eventually(t, func() bool { return logs.FilterMessage("Tunnel ready").Len() == 1 }, time.Second, time.Millisecond)
	second.Close("disconnect")
	third := <-tp.attempts
	require.Equal(t, home.Address, <-tp.addresses)
	require.Eventually(t, func() bool { return logs.FilterMessage("Tunnel recovered").Len() == 1 }, time.Second, time.Millisecond)
	third.Close("disconnect again")
	fourth := <-tp.attempts
	require.Equal(t, home.Address, <-tp.addresses)
	select {
	case err := <-result:
		require.True(t, definite(err))
		require.ErrorContains(t, err, "denied")
	case <-ctx.Done():
		t.Fatal("client did not stop")
	}
	require.Error(t, fourth.Err())
	require.Equal(t, "https://temporary.example.com\n", <-printed)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	service.AssertExpectations(t)
	service.AssertNotCalled(t, "Ping", mock.Anything, mock.Anything)
	pki.AssertExpectations(t)
	require.NotContains(t, strings.Join([]string{l.URL()}, ""), "tg1_")
}

func TestTokenConnections(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expiresAt int64
		expiryLog string
		revoke    bool
	}{
		{
			name:      "cancel",
			expiryLog: "none",
		},
		{
			name:      "revoked",
			expiresAt: time.Date(2030, 2, 3, 4, 5, 6, 0, time.FixedZone("test", -7*60*60)).Unix(),
			expiryLog: "2030-02-03T11:05:06Z",
			revoke:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			core, logs := observer.New(zap.DebugLevel)
			logger := zap.New(core)
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "upstream") }))
			defer target.Close()
			der, _, _ := makeCertificate(require.New(t), logger, &protocol.Node{Id: 1}, &protocol.ClientToken{Token: []byte("certificate")}, nil)
			cert, err := x509.ParseCertificate(der)
			require.NoError(t, err)
			pki := &mocks.PKIClient{}
			pki.On("RequestCertificate", mock.Anything, mock.Anything).Return(&protocol.CertificateResponse{CertDer: der}, nil).Once()
			left, right := mocks.PipeTransport()
			tp := &lightweightTransport{
				MemoryTransport: left,
				cert:            cert,
				addresses:       make(chan string, 32),
				aliases:         map[string]string{"alias-a.example.com:443": "a.example.com:443"},
				connections:     make(map[string]*mocks.PhysicalConn),
			}
			left.On("WithClientCertificate", mock.AnythingOfType("tls.Certificate")).Return(nil).Once()
			service := &mocks.TunnelService{}
			apex := &protocol.Node{Address: "example.com:443"}
			nodes := []*protocol.Node{
				{Address: "a.example.com:443"},
				{Address: "b.example.com:443"},
				{Address: "c.example.com:443"},
			}
			at := func(address string) any {
				return mock.MatchedBy(func(ctx context.Context) bool {
					return rpc.GetDelegation(ctx).Identity.GetAddress() == address
				})
			}
			service.On("GetNodes", at(apex.Address), mock.Anything).Return(&protocol.GetNodesResponse{
				Nodes: []*protocol.Node{
					nodes[0],
					{Address: "alias-a.example.com:443"},
					nodes[1],
				},
			}, nil).Once()
			service.On("GetNodes", mock.Anything, mock.Anything).Return(&protocol.GetNodesResponse{Nodes: nodes}, nil)
			const token = "tg1_secret-that-must-not-appear"
			request := func(slot uint32) any {
				return mock.MatchedBy(func(req *protocol.OpenDelegatedSessionRequest) bool {
					return req.Token == token && req.RouteSlot == slot
				})
			}
			service.On("OpenDelegatedSession", at(apex.Address), request(1)).Return(nil, twirp.ResourceExhausted.Error("apex is full")).Once()
			opened := [3]chan *mocks.PhysicalConn{
				make(chan *mocks.PhysicalConn, 2),
				make(chan *mocks.PhysicalConn, 2),
				make(chan *mocks.PhysicalConn, 2),
			}
			open := func(slot uint32, gate <-chan struct{}) {
				service.On("OpenDelegatedSession", at(nodes[slot-1].Address), request(slot)).
					Run(func(args mock.Arguments) {
						delegation := rpc.GetDelegation(args.Get(0).(context.Context))
						pc := delegation.Conn.(transport.PhysicalConnProvider).PhysicalConn().(*mocks.PhysicalConn)
						opened[slot-1] <- pc
						if gate != nil {
							select {
							case <-gate:
							case <-ctx.Done():
							}
						}
					}).Return(&protocol.OpenSessionResponse{
					Hostname:  "delegated",
					Node:      nodes[slot-1],
					Apex:      "example.com",
					GrantId:   "grant-for-client-test",
					ExpiresAt: tc.expiresAt,
				}, nil).Once()
			}
			allowSecond := make(chan struct{})
			allowRepair := make(chan struct{})
			open(1, nil)
			open(2, allowSecond)
			open(3, nil)
			open(3, allowRepair)
			if tc.revoke {
				service.On("OpenDelegatedSession", at(nodes[0].Address), request(1)).Return(nil, twirp.PermissionDenied.Error("grant revoked")).Once()
			}
			router := transport.NewStreamRouter(logger, nil, right)
			acc := acceptor.NewH2Acceptor(nil)
			defer acc.Close()
			setupRPC(ctx, logger, service, &mocks.KeylessService{}, router, acc)
			go router.Accept(ctx)
			outputR, outputW := io.Pipe()
			defer outputR.Close()
			printed := make(chan string, 8)
			go func() {
				defer close(printed)
				scanner := bufio.NewScanner(outputR)
				for scanner.Scan() {
					printed <- scanner.Text()
				}
			}()
			l, err := NewLightweightClient(LightweightConfig{
				Logger:    logger,
				Transport: tp,
				PKIClient: pki,
				Apex: &dialer.ParsedApex{
					Host: "example.com",
					Port: 443,
				},
				Target: target.URL,
				Token:  token,
				Output: outputW,
			})
			require.NoError(t, err)
			result := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				err := l.Run(ctx)
				outputW.Close()
				result <- err
				close(finished)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-finished:
				case <-time.After(time.Second):
					t.Error("token client did not stop")
				}
			})
			receive := func(slot int) *mocks.PhysicalConn {
				t.Helper()
				select {
				case pc := <-opened[slot-1]:
					return pc
				case <-ctx.Done():
					t.Fatalf("slot %d did not open: %v", slot, logs.All())
					return nil
				}
			}
			first, second := receive(1), receive(2)
			select {
			case line := <-printed:
				require.Equal(t, "https://delegated.example.com", line)
			case <-ctx.Done():
				t.Fatal("URL was not printed with the first attachment")
			}
			require.Equal(t, 1, logs.FilterMessage("Tunnel connection ready").Len())
			var dialed []string
			for len(tp.addresses) > 0 {
				dialed = append(dialed, <-tp.addresses)
			}
			require.Equal(t, apex.Address, dialed[0])
			require.Contains(t, dialed, "alias-a.example.com:443")
			require.NoError(t, first.Err(), "a duplicate physical handle must stay open")
			require.Equal(t, "upstream", lightweightUpstream(t, first, "delegated"))
			close(allowSecond)
			third := receive(3)
			require.Eventually(t, func() bool { return logs.FilterMessage("Tunnel connection ready").Len() == 3 }, time.Second, time.Millisecond)
			require.NotSame(t, first, second)
			require.NotSame(t, first, third)
			require.NotSame(t, second, third)
			third.Close("connection lost")
			replacement := receive(3)
			require.NotSame(t, third, replacement)
			require.NoError(t, first.Err())
			require.NoError(t, second.Err())
			require.Equal(t, "upstream", lightweightUpstream(t, second, "delegated"))
			close(allowRepair)
			require.Eventually(t, func() bool { return logs.FilterMessage("Tunnel connection ready").Len() == 4 }, time.Second, time.Millisecond)
			if tc.revoke {
				first.Close("reconnect after revocation")
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if tc.revoke {
					require.True(t, tokenFatal(err))
					require.ErrorContains(t, err, "grant revoked")
				} else {
					require.NoError(t, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("client did not stop after cancellation or grant rejection")
			}
			for line := range printed {
				t.Errorf("URL printed again: %q", line)
			}
			require.Equal(t, 1, logs.FilterMessage("Tunnel ready").Len())
			slots := map[string]int{
				nodes[0].Address: 1,
				nodes[1].Address: 2,
				nodes[2].Address: 3,
			}
			for _, entry := range logs.FilterMessage("Tunnel connection ready").All() {
				fields := entry.ContextMap()
				require.Equal(t, "grant-for-client-test", fields["grantId"])
				require.Equal(t, tc.expiryLog, fields["expiresAt"])
				require.EqualValues(t, slots[fields["server"].(string)], fields["slot"])
			}
			for _, entry := range logs.All() {
				require.NotContains(t, fmt.Sprint(entry.Message, entry.ContextMap()), token)
			}
			for _, pc := range tp.connections {
				require.Error(t, pc.Err(), "shutdown must close every physical connection")
			}
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
			service.AssertExpectations(t)
			service.AssertNotCalled(t, "Ping", mock.Anything, mock.Anything)
			service.AssertNotCalled(t, "RegisterIdentity", mock.Anything, mock.Anything)
			left.AssertExpectations(t)
			pki.AssertExpectations(t)
		})
	}
}
