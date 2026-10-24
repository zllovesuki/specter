package overlay

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestIntegrationConnectionNegotiation(t *testing.T) {
	if os.Getenv("GO_INTEGRATION_TUNNEL") == "" {
		t.Skip("skipping integration tests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	dial := newReaperTestListener(t, ctx)
	peer := &protocol.Node{Address: "a.example:443"}
	authority := NewQUIC(TransportConfig{
		Logger:           zap.NewNop(),
		Endpoint:         &protocol.Node{Address: "z.example:443"},
		VirtualTransport: true,
	})
	defer authority.Stop()
	key := authority.makeCachedKey(peer)
	type result struct {
		conn *nodeConnection
		err  error
	}
	type candidate struct {
		local, remote *quic.Conn
		control       *quic.Stream
		done          chan result
	}
	start := func(dir direction) candidate {
		t.Helper()
		local, remote := dial()
		if dir == directionIncoming {
			local, remote = remote, local
		}
		stream, err := local.OpenStream()
		require.NoError(t, err)
		done := make(chan result, 1)
		go func() {
			defer WrapQuicConnection(stream, local).Close()
			conn, _, err := authority.reuseConnection(ctx, local, stream, dir)
			done <- result{conn, err}
		}()
		control, err := remote.AcceptStream(ctx)
		require.NoError(t, err)
		require.NoError(t, control.SetDeadline(deadline))
		var identity protocol.Connection
		require.NoError(t, rpc.BoundedReceive(control, &identity, 256))
		require.NoError(t, rpc.Send(control, &protocol.Connection{Identity: peer}))
		return candidate{local, remote, control, done}
	}
	finish := func(c candidate) result {
		t.Helper()
		select {
		case got := <-c.done:
			return got
		case <-ctx.Done():
			t.Fatal("negotiation did not finish")
			return result{}
		}
	}

	first := start(directionOutgoing)
	var status protocol.Connection
	require.NoError(t, rpc.BoundedReceive(first.control, &status, 8))
	require.Equal(t, protocol.Connection_FRESH, status.CacheState)

	// Establish the ownership boundary explicitly, without a timing assumption
	// about how soon the second handshake runs.
	_, pending := authority.negotiations.Load(key)
	require.True(t, pending, "authority must retain arbitration until publication")
	second := start(directionIncoming)
	require.NoError(t, rpc.Send(second.control, &protocol.Connection{
		CacheState: protocol.Connection_FRESH, CacheDirection: protocol.Connection_OUTGOING,
	}))
	require.NoError(t, rpc.Send(first.control, &protocol.Connection{
		CacheState: protocol.Connection_FRESH, CacheDirection: protocol.Connection_INCOMING,
	}))
	winner := finish(first)
	require.NoError(t, winner.err)
	require.NoError(t, rpc.BoundedReceive(second.control, &status, 8))
	require.Equal(t, protocol.Connection_CACHED, status.CacheState)
	require.Equal(t, protocol.Connection_OUTGOING, status.CacheDirection)
	require.ErrorContains(t, finish(second).err, reuseErrorState)
	cached, _ := authority.cachedConnections.Load(key)
	require.Same(t, winner.conn, cached)

	// The first connection still carries traffic after rejecting its competitor.
	stream, err := first.local.OpenStream()
	require.NoError(t, err)
	require.NoError(t, stream.SetDeadline(deadline))
	_, err = stream.Write([]byte("winner"))
	require.NoError(t, err)
	received, err := first.remote.AcceptStream(ctx)
	require.NoError(t, err)
	require.NoError(t, received.SetReadDeadline(deadline))
	payload := make([]byte, len("winner"))
	_, err = io.ReadFull(received, payload)
	require.NoError(t, err)
	require.Equal(t, "winner", string(payload))

	// A later same-direction candidate must not return a cache snapshot that was
	// replaced while its cache-status reply was in flight.
	third := start(directionOutgoing)
	require.NoError(t, rpc.BoundedReceive(third.control, &status, 8))
	require.Equal(t, protocol.Connection_CACHED, status.CacheState)
	replacement := &nodeConnection{peer: peer, quic: second.local, direction: directionIncoming}
	unlock := authority.cachedMutex.Lock(key)
	authority.cachedConnections.Store(key, replacement)
	unlock()
	require.NoError(t, rpc.Send(third.control, &protocol.Connection{
		CacheState: protocol.Connection_CACHED, CacheDirection: protocol.Connection_INCOMING,
	}))
	require.ErrorContains(t, finish(third).err, "cached connection changed")
	cached, _ = authority.cachedConnections.Load(key)
	require.Same(t, replacement, cached)
	require.NoError(t, replacement.quic.Context().Err())
}
