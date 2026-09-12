package server

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
)

func sessionFixture(t *testing.T) (*Server, *mocks.VNode, *mocks.Transport, *x509.Certificate) {
	logger, n, clientT, chordT, s := getFixture(t, require.New(t))
	s.Chord, s.ParentContext = n, t.Context()
	cli, ch, tn := getIdentities()
	n.On("ID").Return(ch.Id).Maybe()
	clientT.On("Identity").Return(tn)
	chordT.On("Identity").Return(ch)
	cert := toCertificate(require.New(t), logger, cli, &protocol.ClientToken{Token: mustGenerateToken()})
	t.Cleanup(func() { s.rpcAcceptor.Close(); s.routeCache.Close(); s.keylessCache.Close() })
	return s, n, clientT, cert
}

func sessionContext(t *testing.T, cert *x509.Certificate) (context.Context, *mocks.PhysicalConn) {
	pc := mocks.NewPhysicalConn(func(d *transport.StreamDelegate) { d.Close() })
	t.Cleanup(func() { pc.Close("test cleanup") })
	conn, err := pc.OpenStream(protocol.Stream_RPC)
	require.NoError(t, err)
	return rpc.WithDelegation(t.Context(), &transport.StreamDelegate{
		Conn:        conn,
		Certificate: cert,
	}), pc
}

func TestOpenEphemeralSession(t *testing.T) {
	s, n, clientT, cert := sessionFixture(t)
	tp := mocks.SelfTransport()
	tp.WithCertificate(cert)
	delivered := make(chan *transport.StreamDelegate, 1)
	pc := mocks.NewPhysicalConn(func(d *transport.StreamDelegate) { delivered <- d })
	t.Cleanup(func() { pc.Close("cleanup") })
	tp.Physical = pc
	router := transport.NewStreamRouter(s.Logger, nil, tp)
	s.AttachRouter(t.Context(), router)
	go router.Accept(t.Context())
	ctx := t.Context()
	httpClient := &http.Client{Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return tp.DialStream(ctx, &protocol.Node{Address: "test"}, protocol.Stream_RPC)
		},
	}}
	cli := protocol.NewTunnelServiceProtobufClient("http://tunnel", httpClient)
	// Open methods authenticate their attachment without owner registration.
	tp.WithCertificate(nil)
	_, err := cli.OpenEphemeralSession(ctx, &protocol.OpenEphemeralSessionRequest{})
	require.Equal(t, twirp.Unauthenticated, err.(twirp.Error).Code())
	_, err = cli.OpenDelegatedSession(ctx, &protocol.OpenDelegatedSessionRequest{})
	require.Equal(t, twirp.Unauthenticated, err.(twirp.Error).Code())
	tp.WithCertificate(cert)
	resp, err := cli.OpenEphemeralSession(ctx, &protocol.OpenEphemeralSessionRequest{})
	require.NoError(t, err)
	expected, _, err := tun.EphemeralLabel(s.Chord.ID(), cert.RawSubjectPublicKeyInfo)
	require.NoError(t, err)
	require.Equal(t, expected, resp.Hostname)
	require.Equal(t, s.TunnelTransport.Identity(), resp.Node)
	require.Equal(t, s.Apex, resp.Apex)
	conn, err := s.DialClient(t.Context(), &protocol.Link{
		Hostname: expected,
		Alpn:     protocol.Link_HTTP,
	})
	require.NoError(t, err)
	conn.Close()
	(<-delivered).Close()
	_, err = cli.OpenEphemeralSession(ctx, &protocol.OpenEphemeralSessionRequest{})
	require.Equal(t, twirp.FailedPrecondition, err.(twirp.Error).Code())
	require.NoError(t, pc.Err())
	pc.Close("disconnected")
	_, err = s.DialClient(t.Context(), &protocol.Link{Hostname: expected})
	require.ErrorIs(t, err, tun.ErrTunnelClientNotConnected)
	clientT.AssertNotCalled(t, "DialStream", mock.Anything, mock.Anything, mock.Anything)
	nextCtx, _ := sessionContext(t, cert)
	reopened, err := s.OpenEphemeralSession(nextCtx, &protocol.OpenEphemeralSessionRequest{})
	require.NoError(t, err)
	require.Equal(t, expected, reopened.Hostname)
	_, _, _, another := sessionFixture(t)
	nextCtx, _ = sessionContext(t, another)
	fresh, err := s.OpenEphemeralSession(nextCtx, &protocol.OpenEphemeralSessionRequest{})
	require.NoError(t, err)
	require.NotEqual(t, expected, fresh.Hostname)
	for _, method := range []string{"Put", "PrefixAppend", "Delete", "Acquire"} {
		n.AssertNotCalled(t, method, mock.Anything, mock.Anything, mock.Anything)
	}
}

func TestOpenDelegatedSession(t *testing.T) {
	for _, scenario := range []string{"slot 1", "slot 2", "slot 3", "custom valid", "malformed", "unknown", "version", "expired", "not owned", "custom mismatch", "publication failed"} {
		t.Run(scenario, func(t *testing.T) {
			s, n, _, cert := sessionFixture(t)
			ctx, pc := sessionContext(t, cert)
			secret := [32]byte{7}
			token := tun.EncodeDelegationToken(secret)
			rec := &protocol.DelegationRecord{
				Version:  1,
				Id:       tun.DelegationID(secret),
				Owner:    &protocol.ClientToken{Token: []byte("owner")},
				Hostname: "test-host",
			}
			code := twirp.NoError
			slot := uint32(1)
			switch scenario {
			case "slot 2":
				slot = 2
				rec.ExpiresAt = time.Now().Add(time.Hour).Unix()
			case "slot 3":
				slot = 3
			case "malformed":
				token = "bad"
				code = twirp.InvalidArgument
			case "unknown":
				code = twirp.Unauthenticated
			case "version":
				rec.Version = 2
				code = twirp.Unauthenticated
			case "expired":
				rec.ExpiresAt = time.Now().Add(-time.Hour).Unix()
				code = twirp.PermissionDenied
			case "not owned", "custom mismatch":
				code = twirp.PermissionDenied
			case "publication failed":
				code = twirp.Unavailable
			}
			if scenario == "custom mismatch" || scenario == "custom valid" {
				rec.Hostname = "test.example.com"
			}
			if scenario != "malformed" {
				data, err := rec.MarshalVT()
				require.NoError(t, err)
				if scenario == "unknown" {
					data = nil
				}
				n.On("Get", mock.Anything, []byte(tun.DelegationKey(rec.Id))).Return(data, nil).Once()
			}
			if code == twirp.NoError || scenario == "publication failed" || scenario == "not owned" || scenario == "custom mismatch" {
				acquired := n.On("Acquire", mock.Anything, []byte(tun.ClientLeaseKey(rec.Owner)), 30*time.Second).Return(uint64(1), nil).Once()
				n.On("Release", mock.Anything, []byte(tun.ClientLeaseKey(rec.Owner)), uint64(1)).Return(nil).Once().NotBefore(acquired)
				n.On("PrefixContains", mock.Anything, []byte(tun.ClientHostnamesPrefix(rec.Owner)), []byte(rec.Hostname)).Return(scenario != "not owned", nil).Once()
				if scenario == "custom mismatch" || scenario == "custom valid" {
					owner := rec.Owner
					if scenario == "custom mismatch" {
						owner = &protocol.ClientToken{Token: []byte("someone else")}
					}
					data, _ := (&protocol.CustomHostname{ClientToken: owner}).MarshalVT()
					n.On("Get", mock.Anything, []byte(tun.CustomHostnameKey(rec.Hostname))).Return(data, nil).Once()
				}
			}
			var written []*protocol.TunnelRoute
			if code == twirp.NoError || scenario == "publication failed" {
				var putErr error
				if scenario == "publication failed" {
					putErr = errors.New("storage unavailable")
				}
				n.On("Put", mock.Anything, []byte(tun.RoutingKey(rec.Hostname, int(slot))), mock.Anything).Run(func(args mock.Arguments) {
					route := &protocol.TunnelRoute{}
					require.NoError(t, route.UnmarshalVT(args.Get(2).([]byte)))
					written = append(written, route)
				}).Return(putErr).Once()
			}
			resp, err := s.OpenDelegatedSession(ctx, &protocol.OpenDelegatedSessionRequest{
				Token:     token,
				RouteSlot: slot,
			})
			if code == twirp.NoError {
				require.NoError(t, err)
				require.Equal(t, rec.Id, resp.GrantId)
				require.Equal(t, rec.ExpiresAt, resp.ExpiresAt)
				require.Equal(t, rec.Hostname, resp.Hostname)
				require.Equal(t, s.TunnelTransport.Identity(), resp.Node)
				require.Equal(t, s.Apex, resp.Apex)
				require.Len(t, written, 1)
				require.True(t, tun.IsSessionAlias(written[0].ClientDestination.Address))
				require.True(t, written[0].ClientDestination.Rendezvous)
				require.Equal(t, s.ChordTransport.Identity(), written[0].ChordDestination)
				require.Equal(t, s.TunnelTransport.Identity(), written[0].TunnelDestination)
				conn, err := s.sessions.dial(written[0].ClientDestination.Address, rec.Hostname)
				require.NoError(t, err)
				require.Equal(t, pc, conn.(transport.PhysicalConnProvider).PhysicalConn())
				conn.Close()
			} else {
				require.Equal(t, code, err.(twirp.Error).Code())
				if len(written) != 0 {
					_, err := s.sessions.dial(written[0].ClientDestination.Address, rec.Hostname)
					require.ErrorIs(t, err, transport.ErrNoDirect)
				}
				require.Eventually(t, func() bool { return connectionDone(pc) }, 2*time.Second, 10*time.Millisecond)
			}
			n.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
			n.AssertExpectations(t)
		})
	}
}

func TestOpenDelegatedSessionRejectsInvalidSlot(t *testing.T) {
	s, n, _, cert := sessionFixture(t)
	ctx, pc := sessionContext(t, cert)
	for _, slot := range []uint32{0, tun.NumRedundantLinks + 1} {
		_, err := s.OpenDelegatedSession(ctx, &protocol.OpenDelegatedSessionRequest{RouteSlot: slot})
		require.Equal(t, twirp.InvalidArgument, err.(twirp.Error).Code())
	}
	s.sessions.mu.Lock()
	reserved := len(s.sessions.byConn)
	s.sessions.mu.Unlock()
	require.Zero(t, reserved)
	require.NoError(t, pc.Err())
	require.Empty(t, n.Calls)
}

func TestSessionCleanupPreservesReboundAlias(t *testing.T) {
	registry := newSessionRegistry()
	physical := func() *mocks.PhysicalConn {
		conn := mocks.NewPhysicalConn(func(d *transport.StreamDelegate) { d.Close() })
		t.Cleanup(func() { conn.Close("test cleanup") })
		return conn
	}
	old := &session{
		alias:    tun.SessionAlias([16]byte{1}),
		hostname: "test",
		conn:     physical(),
		mode:     ephemeral,
		spki:     []byte("same key"),
	}
	require.NoError(t, registry.reserve(old))
	// Model failed setup awaiting physical closure while its error response flushes.
	old.mu.Lock()
	old.state = terminal
	old.mu.Unlock()
	replacement := &session{
		alias:    old.alias,
		hostname: old.hostname,
		conn:     physical(),
		mode:     ephemeral,
		spki:     old.spki,
	}
	require.NoError(t, registry.reserve(replacement))
	require.True(t, replacement.activate(t.Context(), time.Time{}))
	old.conn.Close("old attachment ended")
	require.Eventually(t, func() bool {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		return registry.byConn[old.conn] == nil
	}, time.Second, time.Millisecond)

	conn, err := registry.dial(replacement.alias, replacement.hostname)
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, replacement.conn, conn.(transport.PhysicalConnProvider).PhysicalConn())
}
