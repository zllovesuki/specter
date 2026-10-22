package overlay

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
)

func TestQuicPipeTCPBackpressure(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	sourceQ, gatewayQ := newReaperTestListener(t, ctx)()
	gatewayStream, err := gatewayQ.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = gatewayStream.Write([]byte("poke"))
	require.NoError(t, err)
	sourceStream, err := sourceQ.AcceptStream(ctx)
	require.NoError(t, err)
	_, err = io.ReadFull(sourceStream, make([]byte, 4))
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	consumer, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer consumer.Close()
	require.NoError(t, consumer.SetDeadline(time.Now().Add(15*time.Second)))
	require.NoError(t, consumer.(*net.TCPConn).SetReadBuffer(4096))
	gatewayTCP, err := listener.Accept()
	require.NoError(t, err)
	defer gatewayTCP.Close()
	require.NoError(t, gatewayTCP.(*net.TCPConn).SetWriteBuffer(4096))
	gatewayDone := tun.Pipe(WrapQuicConnection(gatewayStream, gatewayQ), gatewayTCP)

	upstream, origin := net.Pipe()
	defer upstream.Close()
	defer origin.Close()
	// Production tunnel handlers pass the stream through this delegate.
	source := &transport.StreamDelegate{Conn: WrapQuicConnection(sourceStream, sourceQ)}
	sourceDone := tun.Pipe(source, upstream)
	payload := bytes.Repeat([]byte("response"), 32768)
	originDone := make(chan error, 1)
	go func() {
		defer origin.Close()
		_, err := io.ReadFull(origin, make([]byte, len("request")))
		if err == nil {
			_, err = origin.Write(payload)
		}
		originDone <- err
	}()
	_, err = consumer.Write([]byte("request"))
	require.NoError(t, err)
	require.NoError(t, <-originDone)
	for range sourceDone {
	}

	// The upstream has finished, but TCP backpressure keeps response bytes
	// queued at the gateway past the former one-second reset boundary.
	time.Sleep(1500 * time.Millisecond)
	got, err := io.ReadAll(consumer)
	require.NoError(t, err)
	require.Equal(t, len(payload), len(got))
	require.Equal(t, payload, got)
	for range gatewayDone {
	}
}

func TestQuicPipeCloseUnblocksStoppedReader(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	sourceQ, peerQ := newReaperTestListener(t, ctx)()
	peerStream, err := peerQ.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = peerStream.Write([]byte("poke"))
	require.NoError(t, err)
	sourceStream, err := sourceQ.AcceptStream(ctx)
	require.NoError(t, err)
	_, err = io.ReadFull(sourceStream, make([]byte, 4))
	require.NoError(t, err)
	source := WrapQuicConnection(sourceStream, sourceQ)
	upstream, origin := net.Pipe()
	defer upstream.Close()
	defer origin.Close()
	pipeErrors := tun.Pipe(source, upstream)
	pipeDone := make(chan struct{})
	go func() {
		for range pipeErrors {
		}
		close(pipeDone)
	}()
	writeDone := make(chan error, 1)
	go func() {
		// More than the peer's initial receive window; it never reads.
		_, err := origin.Write(make([]byte, 1<<20))
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		t.Fatalf("write finished without a reader: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// Explicit Close must still wake both copies and cancel the blocked send.
	require.NoError(t, source.Close())
	select {
	case <-pipeDone:
	case <-ctx.Done():
		t.Fatal("Close did not release the blocked pipe")
	}
	require.Error(t, <-writeDone)
	require.NoError(t, peerStream.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = io.Copy(io.Discard, peerStream)
	var reset *quic.StreamError
	require.ErrorAs(t, err, &reset)
	require.EqualValues(t, 409, reset.ErrorCode)
	require.True(t, reset.Remote)
	require.NoError(t, sourceQ.Context().Err())
	require.NoError(t, peerQ.Context().Err())
}
