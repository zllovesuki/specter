package overlay

import (
	"context"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"

	"github.com/stretchr/testify/require"
)

func TestAttachmentBoundToPhysicalConn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	dial := newReaperTestListener(t, ctx)
	client, server := dial()
	stream, err := openStream(client, &protocol.Stream{Type: protocol.Stream_RPC})
	require.NoError(t, err)
	defer stream.Close()
	incoming, err := server.AcceptStream(ctx)
	require.NoError(t, err)
	conn := WrapQuicConnection(incoming, server)
	defer conn.Close()
	var header protocol.Stream
	require.NoError(t, rpc.BoundedReceive(conn, &header, 1024))
	pc := conn.(transport.PhysicalConnProvider).PhysicalConn()
	direct, err := pc.OpenStream(protocol.Stream_DIRECT)
	require.NoError(t, err)
	defer direct.Close()
	received, err := client.AcceptStream(ctx)
	require.NoError(t, err)
	require.NoError(t, rpc.BoundedReceive(received, &header, 1024))
	require.Equal(t, protocol.Stream_DIRECT, header.Type)
	require.NoError(t, client.CloseWithError(0, "test disconnect"))
	select {
	case <-pc.Done():
	case <-ctx.Done():
		t.Fatal("attachment did not close")
	}
	_, replacement := dial()
	require.NotEqual(t, pc, transport.PhysicalConn(attachment{replacement}))
	_, err = pc.OpenStream(protocol.Stream_DIRECT)
	require.Error(t, err)
	require.Error(t, pc.Err())
}
