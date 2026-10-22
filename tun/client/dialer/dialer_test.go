package dialer

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/libp2p/go-yamux/v4"
	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDialerReconnect(t *testing.T) {
	fixture := httptest.NewTLSServer(nil)
	cert := fixture.TLS.Certificates[0]
	fixture.Close()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{tun.ALPN(protocol.Link_TCP)},
	}

	for _, tcp := range []bool{true, false} {
		name, dial := "quic", QuicDialer
		if tcp {
			name, dial = "tls-yamux", TLSDialer
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			addr, peers := listenDialerPeers(t, ctx, tlsConfig, tcp)
			parsed, err := ParseApex(addr.String())
			require.NoError(t, err)
			core, logs := observer.New(zap.InfoLevel)
			_, d, err := dial(ctx, DialerConfig{
				Logger:             zap.New(core),
				Parsed:             parsed,
				InsecureSkipVerify: true,
			})
			require.NoError(t, err)

			first := receiveDialerPeer(t, ctx, peers)
			checkDialerStream(t, d, first)
			first.close()
			second := receiveDialerPeer(t, ctx, peers)
			require.Eventually(t, func() bool {
				return logs.FilterMessage("Connection to gateway re-established").Len() == 1
			}, time.Second, time.Millisecond)
			checkDialerStream(t, d, second)
		})
	}
}

type dialerPeer struct {
	accept func() (io.ReadWriteCloser, error)
	close  func()
}

func listenDialerPeers(t *testing.T, ctx context.Context, cfg *tls.Config, tcp bool) (net.Addr, <-chan dialerPeer) {
	t.Helper()
	peers := make(chan dialerPeer, 2)
	if tcp {
		listener, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
		require.NoError(t, err)
		t.Cleanup(func() { listener.Close() })
		go func() {
			for range 2 {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				config := yamux.DefaultConfig()
				config.LogOutput = io.Discard
				session, err := yamux.Server(conn, config, nil)
				if err != nil {
					conn.Close()
					return
				}
				closePeer := func() { session.Close() }
				context.AfterFunc(ctx, closePeer)
				peers <- dialerPeer{
					accept: func() (io.ReadWriteCloser, error) { return session.AcceptStream() },
					close:  closePeer,
				}
			}
		}()
		return listener.Addr(), peers
	}
	listener, err := quic.ListenAddr("127.0.0.1:0", cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	go func() {
		for range 2 {
			conn, err := listener.Accept(ctx)
			if err != nil {
				return
			}
			closePeer := func() { conn.CloseWithError(0, "test disconnect") }
			context.AfterFunc(ctx, closePeer)
			peers <- dialerPeer{
				accept: func() (io.ReadWriteCloser, error) { return conn.AcceptStream(ctx) },
				close:  closePeer,
			}
		}
	}()
	return listener.Addr(), peers
}

func receiveDialerPeer(t *testing.T, ctx context.Context, peers <-chan dialerPeer) dialerPeer {
	t.Helper()
	select {
	case peer := <-peers:
		return peer
	case <-ctx.Done():
		t.Fatal("timed out waiting for gateway connection")
		return dialerPeer{}
	}
}

func checkDialerStream(t *testing.T, d TransportDialer, peer dialerPeer) {
	t.Helper()
	conn, err := d.Dial()
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	_, err = conn.Write([]byte("request"))
	require.NoError(t, err)
	stream, err := peer.accept()
	require.NoError(t, err)
	defer stream.Close()
	data := make([]byte, len("request"))
	_, err = io.ReadFull(stream, data)
	require.NoError(t, err)
	require.Equal(t, "request", string(data))
	_, err = stream.Write([]byte("response"))
	require.NoError(t, err)
	data = make([]byte, len("response"))
	_, err = io.ReadFull(conn, data)
	require.NoError(t, err)
	require.Equal(t, "response", string(data))
}
