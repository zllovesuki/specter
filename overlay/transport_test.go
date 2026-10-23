package overlay

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type connectedTestDialer struct {
	conn    *quic.Conn
	started chan struct{}
}

func (d connectedTestDialer) DialEarly(context.Context, net.Addr, *tls.Config, *quic.Config) (*quic.Conn, error) {
	if d.started != nil {
		close(d.started)
	}
	return d.conn, nil
}

func TestCachedConnectionOutlivesFirstDialContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	localQ, remoteQ := newReaperTestListener(t, ctx, &quic.Config{EnableDatagrams: true})()
	local := NewQUIC(TransportConfig{
		Logger:           zap.NewNop(),
		QuicTransport:    connectedTestDialer{conn: localQ},
		ClientTLS:        &tls.Config{},
		Endpoint:         &protocol.Node{Address: localQ.LocalAddr().String()},
		VirtualTransport: true,
	})
	defer local.Stop()
	remote := NewQUIC(TransportConfig{
		Logger:           zap.NewNop(),
		Endpoint:         &protocol.Node{Address: remoteQ.LocalAddr().String()},
		VirtualTransport: true,
	})
	defer remote.Stop()
	accepted := make(chan error, 1)
	go func() {
		_, err := remote.handleIncoming(ctx, remoteQ)
		accepted <- err
	}()

	dialCtx, finishExchange := context.WithCancel(ctx)
	defer finishExchange()
	first, err := local.DialStream(dialCtx, remote.Identity(), protocol.Stream_CONFIG)
	require.NoError(t, err)
	require.NoError(t, <-accepted)
	select {
	case delegate := <-remote.AcceptStream():
		require.Equal(t, protocol.Stream_CONFIG, delegate.Kind)
		delegate.Close()
	case <-ctx.Done():
		t.Fatal("initial configuration stream did not arrive")
	}
	require.NoError(t, first.Close())
	finishExchange()
	require.NoError(t, local.backgroundCtx.Err(), "a completed dial must not cancel transport-wide workers")

	// Finishing the first CONFIG exchange must not stop the shared peer's
	// reverse RPC or datagram receivers. Both directions use this same cache.
	for range 2 {
		stream, err := remote.DialStream(ctx, local.Identity(), protocol.Stream_RPC)
		require.NoError(t, err)
		defer stream.Close()
		_, err = stream.Write([]byte("rpc"))
		require.NoError(t, err)
		select {
		case delegate := <-local.AcceptStream():
			require.Equal(t, protocol.Stream_RPC, delegate.Kind)
			data := make([]byte, 3)
			_, err := io.ReadFull(delegate, data)
			require.NoError(t, err)
			require.Equal(t, "rpc", string(data))
			delegate.Close()
		case <-ctx.Done():
			t.Fatal("finishing the CONFIG dial stopped cached peer stream reception")
		}
	}
	require.NoError(t, remote.SendDatagram(local.Identity(), []byte("datagram")))
	select {
	case datagram := <-local.ReceiveDatagram():
		require.Equal(t, []byte("datagram"), datagram.Buffer)
	case <-ctx.Done():
		t.Fatal("finishing the CONFIG dial stopped cached peer datagram reception")
	}
	require.Len(t, local.ListConnected(), 1)
	require.Len(t, remote.ListConnected(), 1)
	require.Equal(t, transport.PhysicalConn(attachment{localQ}), local.ListConnected()[0].Physical)
	local.Stop()
	require.ErrorIs(t, local.backgroundCtx.Err(), context.Canceled)
	select {
	case <-localQ.Context().Done():
	case <-ctx.Done():
		t.Fatal("transport stop did not close the cached peer")
	}
}

func TestCanceledConnectionSetupClosesPendingPeer(t *testing.T) {
	for _, stopTransport := range []bool{false, true} {
		name := "dial canceled"
		if stopTransport {
			name = "transport stopped"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			localQ, remoteQ := newReaperTestListener(t, ctx)()
			started := make(chan struct{})
			local := NewQUIC(TransportConfig{
				Logger:           zap.NewNop(),
				QuicTransport:    connectedTestDialer{conn: localQ, started: started},
				ClientTLS:        &tls.Config{},
				Endpoint:         &protocol.Node{Address: localQ.LocalAddr().String()},
				VirtualTransport: true,
			})
			defer local.Stop()
			dialCtx, stopDial := context.WithCancel(ctx)
			defer stopDial()
			done := make(chan error, 1)
			// The peer completed TLS but never starts overlay negotiation.
			go func() {
				_, err := local.DialStream(dialCtx, &protocol.Node{Address: remoteQ.LocalAddr().String()}, protocol.Stream_CONFIG)
				done <- err
			}()
			<-started
			if stopTransport {
				local.Stop()
			} else {
				stopDial()
			}
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("cancellation did not interrupt connection setup")
			}
			select {
			case <-localQ.Context().Done():
			case <-time.After(time.Second):
				t.Fatal("cancellation left an uncached QUIC connection open")
			}
		})
	}
}
