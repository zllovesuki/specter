package client

import (
	"context"
	"net"
	"time"

	"specter/spec/protocol"
	"specter/spec/rpc"
	"specter/spec/transport"
	"specter/spec/tun"

	"go.uber.org/zap"
)

// Tunnel create procedure:
// client connects to server with random ID (TODO: use delegation so we can generate it)
// Transport exchanges identities and notify us as part of transport.Delegate
// at this point, we have a bidirectional Transport with client
// client then should request X more nodes from us, say 2 successors
// client then connects to X more nodes
// all those nodes should also have bidrectional Transport with client
// client then issue RPC to save 3 copies of (ClientIdentity, ServerIdentity) to DHT
// keys: chordHash(hostname-[1..3])
// tunnel is now registered

type Client struct {
	logger          *zap.Logger
	serverTransport transport.Transport
	rpc             rpc.RPC
}

func NewClient(ctx context.Context, logger *zap.Logger, t transport.Transport, server *protocol.Node) (*Client, error) {
	r, err := t.DialRPC(ctx, server, nil)
	if err != nil {
		return nil, err
	}
	c := &Client{
		logger:          logger,
		serverTransport: t,
		rpc:             r,
	}

	return c, nil
}

func (c *Client) Accept(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delegate := <-c.serverTransport.Direct():
			go c.forward(ctx, delegate.Connection)
		}
	}
}

func (c *Client) forward(ctx context.Context, remote net.Conn) {
	dialer := &net.Dialer{
		Timeout: time.Second * 3,
	}
	local, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:8080")
	if err != nil {
		c.logger.Error("forwarding connection", zap.Error(err))
		remote.Close()
		return
	}
	tun.Pipe(remote, local)
}
