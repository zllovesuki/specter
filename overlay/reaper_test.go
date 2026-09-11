package overlay

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"testing"
	"time"

	rttinstrumentation "go.miragespace.co/specter/rtt"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap/zaptest"
)

func TestReapPeerPreservesReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	dial := newReaperTestListener(t, ctx)
	old, _ := dial()
	replacement, remote := dial()
	peer := &protocol.Node{Address: replacement.RemoteAddr().String(), Id: 1}
	recorder := rttinstrumentation.NewInstrumentation(16)
	transport := NewQUIC(TransportConfig{
		Logger:      zaptest.NewLogger(t),
		RTTRecorder: recorder,
	})
	key := transport.makeCachedKey(peer)

	// Cache a replacement before the old connection's delayed cleanup.
	current := &nodeConnection{peer: peer, quic: replacement}
	transport.cachedConnections.Store(key, current)
	pending := skipmap.NewUint64[int64]()
	pending.Store(1, time.Now().UnixNano())
	transport.rttMap.Store(key, pending)
	measurementKey := rtt.MakeMeasurementKey(peer)
	recorder.RecordSent(measurementKey)
	recorder.RecordLatency(measurementKey, float64(time.Millisecond))
	before := recorder.Snapshot(measurementKey, time.Minute)
	require.NotNil(t, before)

	transport.reapPeer(old, peer)

	cached, ok := transport.cachedConnections.Load(key)
	require.True(t, ok, "stale cleanup removed the replacement")
	require.Same(t, current, cached)
	require.NoError(t, replacement.Context().Err(), "stale cleanup closed the replacement")
	require.Error(t, old.Context().Err(), "cleanup must close its own connection")
	mapping, ok := transport.rttMap.Load(key)
	require.True(t, ok, "stale cleanup removed the replacement's pending RTT probes")
	require.Same(t, pending, mapping)
	require.Equal(t, before, recorder.Snapshot(measurementKey, time.Minute))

	// The cached replacement must still carry traffic, not just remain
	// present in the cache.
	outgoing, err := replacement.OpenStreamSync(ctx)
	require.NoError(t, err)
	deadline, _ := ctx.Deadline()
	require.NoError(t, outgoing.SetWriteDeadline(deadline))
	_, err = outgoing.Write([]byte("replacement"))
	require.NoError(t, err)
	require.NoError(t, outgoing.Close())
	incoming, err := remote.AcceptStream(ctx)
	require.NoError(t, err)
	require.NoError(t, incoming.SetReadDeadline(deadline))
	payload, err := io.ReadAll(incoming)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(payload))

	transport.reapPeer(replacement, peer)

	_, ok = transport.cachedConnections.Load(key)
	require.False(t, ok, "current cleanup must remove the cached connection")
	require.Error(t, replacement.Context().Err())
	_, ok = transport.rttMap.Load(key)
	require.False(t, ok, "current cleanup must remove pending RTT probes")
	require.Nil(t, recorder.Snapshot(measurementKey, time.Minute))
}

func newReaperTestListener(t *testing.T, ctx context.Context) func() (*quic.Conn, *quic.Conn) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	listener, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: privateKey}},
		NextProtos:   []string{"specter-reaper-test"},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	return func() (*quic.Conn, *quic.Conn) {
		t.Helper()
		local, err := quic.DialAddr(ctx, listener.Addr().String(), &tls.Config{
			ServerName: "localhost",
			RootCAs:    roots,
			NextProtos: []string{"specter-reaper-test"},
		}, nil)
		require.NoError(t, err)
		t.Cleanup(func() { local.CloseWithError(0, "test cleanup") })
		remote, err := listener.Accept(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { remote.CloseWithError(0, "test cleanup") })
		return local, remote
	}
}
