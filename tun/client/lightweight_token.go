package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util"

	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

type tokenConnection struct {
	conn transport.PhysicalConn
	node *protocol.Node
}

// Only the reconnect callback changes slots and candidates. Connection watchers
// send coalesced notifications; they never mutate connection or publication state.
type tokenConnections struct {
	client     *LightweightClient
	slots      [tun.NumRedundantLinks]*tokenConnection
	candidates []*protocol.Node
	wake       chan struct{}
	backoff    time.Duration
}

func (l *LightweightClient) runToken(ctx context.Context) error {
	c := &tokenConnections{
		client:  l,
		wake:    make(chan struct{}, 1),
		backoff: time.Second,
	}
	defer func() {
		for slot, connection := range c.slots {
			if connection != nil {
				connection.conn.Close("token tunnel ended")
				c.remove(slot)
			}
		}
	}()
	c.wake <- struct{}{}
	return runReconnectLoop(ctx, c.wake, c.reconcile)
}

func (c *tokenConnections) reconcile(ctx context.Context) (time.Duration, error) {
	c.prune()
	if c.vacancy() < 0 {
		return 0, nil
	}
	apex := &protocol.Node{Address: c.client.Apex.String()}
	var bootstrap transport.PhysicalConn
	tried := make(map[string]bool)
	attempts, unsupportedCount := 0, 0
	if c.count() == 0 {
		stream, pc, err := c.client.dialAttachment(ctx, apex)
		if err != nil {
			if tokenFatal(err) {
				return 0, err
			}
			tried[apex.Address], attempts = true, 1
			c.client.Logger.Warn("Bootstrap connection failed", zap.Error(err))
		} else {
			bootstrap = pc
			defer func() {
				if !c.hasConnection(pc, "") {
					pc.Close("bootstrap attempt ended")
				}
			}()
			// Discovery does not depend on session admission. Reuse this physical
			// connection for Open even when discovery itself is unavailable.
			if err := c.discover(ctx, stream); err != nil {
				c.client.Logger.Warn("Server discovery failed", zap.Error(err))
			}
		}
	} else {
		for _, connection := range c.slots {
			if connection == nil {
				continue
			}
			stream, err := connection.conn.OpenStream(protocol.Stream_RPC)
			if err == nil {
				err = c.discover(ctx, stream)
			}
			if err == nil {
				break
			}
			c.client.Logger.Warn("Server discovery failed", zap.String("server", connection.node.GetAddress()), zap.Error(err))
		}
	}
	candidates := append(append([]*protocol.Node{}, c.candidates...), apex)
	if bootstrap != nil {
		candidates = append([]*protocol.Node{apex}, candidates...)
	}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		c.prune()
		slot := c.vacancy()
		if slot < 0 {
			break
		}
		address := candidate.GetAddress()
		if tried[address] || c.hasConnection(nil, address) {
			continue
		}
		tried[address] = true
		attempts++
		pc := bootstrap
		var stream net.Conn
		var err error
		if pc != nil && address == apex.Address {
			stream, err = pc.OpenStream(protocol.Stream_RPC)
		} else {
			stream, pc, err = c.client.dialAttachment(ctx, candidate)
		}
		if err != nil {
			if tokenFatal(err) {
				return 0, err
			}
			c.client.Logger.Warn("Tunnel connection failed", zap.String("server", address), zap.Error(err))
			continue
		}
		// An apex alias and a discovered address may share the transport's cached
		// connection. Never issue another Open or close an already owned handle.
		if c.hasConnection(pc, "") {
			stream.Close()
			continue
		}
		if err := c.open(ctx, stream, pc, slot); err != nil {
			pc.Close("token attachment failed")
			if tokenFatal(err) {
				return 0, err
			}
			if unsupported(err) {
				unsupportedCount++
			}
			c.client.Logger.Warn("Tunnel connection failed", zap.String("server", address), zap.Error(err))
		}
	}
	c.prune()
	if c.count() > 0 {
		c.backoff = time.Second
		return 0, nil
	}
	if attempts > 0 && attempts == unsupportedCount {
		return 0, fmt.Errorf("servers do not support lightweight tunnels")
	}
	delay := util.RandomTimeRange(c.backoff)
	c.backoff = min(2*c.backoff, 30*time.Second)
	c.client.Logger.Warn("Retrying token tunnel", zap.Duration("backoff", delay))
	return delay, nil
}

func (c *tokenConnections) discover(ctx context.Context, stream net.Conn) error {
	defer stream.Close()
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	resp, err := tunnelClientOnStream(stream).GetNodes(ctx, &protocol.GetNodesRequest{})
	if err != nil {
		return err
	}
	nodes := make([]*protocol.Node, 0, tun.NumRedundantLinks)
	seen := make(map[string]bool)
	for _, node := range resp.GetNodes() {
		address := node.GetAddress()
		if address == "" || seen[address] {
			continue
		}
		seen[address] = true
		nodes = append(nodes, node)
		if len(nodes) == tun.NumRedundantLinks {
			break
		}
	}
	c.candidates = nodes
	return nil
}

func (c *tokenConnections) open(ctx context.Context, stream net.Conn, pc transport.PhysicalConn, slot int) error {
	defer stream.Close()
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := tunnelClientOnStream(stream).OpenDelegatedSession(callCtx, &protocol.OpenDelegatedSessionRequest{
		Token:     c.client.Token,
		RouteSlot: uint32(slot + 1),
	})
	if err != nil {
		return err
	}
	c.prune()
	if c.hasConnection(nil, resp.GetNode().GetAddress()) {
		return fmt.Errorf("server is already attached")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case <-pc.Done():
		return fmt.Errorf("connection ended during activation: %v", pc.Err())
	default:
	}
	if err := c.client.acceptSession(resp); err != nil {
		return err
	}
	c.slots[slot] = &tokenConnection{
		conn: pc,
		node: resp.GetNode(),
	}
	expiry := "none"
	if resp.GetExpiresAt() != 0 {
		expiry = time.Unix(resp.GetExpiresAt(), 0).UTC().Format(time.RFC3339)
	}
	c.client.Logger.Info("Tunnel connection ready", zap.Int("slot", slot+1), zap.String("server", resp.GetNode().GetAddress()), zap.String("grantId", resp.GetGrantId()), zap.String("expiresAt", expiry))
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-pc.Done():
		}
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}()
	return nil
}

func (c *tokenConnections) prune() {
	for slot, connection := range c.slots {
		if connection != nil {
			select {
			case <-connection.conn.Done():
				c.remove(slot)
			default:
			}
		}
	}
}

func (c *tokenConnections) remove(slot int) {
	connection := c.slots[slot]
	c.slots[slot] = nil
	c.client.Logger.Info("Tunnel disconnected", zap.Int("slot", slot+1), zap.String("server", connection.node.GetAddress()), zap.Error(connection.conn.Err()))
}

func (c *tokenConnections) vacancy() int {
	for slot, connection := range c.slots {
		if connection == nil {
			return slot
		}
	}
	return -1
}

func (c *tokenConnections) count() int {
	count := 0
	for _, connection := range c.slots {
		if connection != nil {
			count++
		}
	}
	return count
}

func (c *tokenConnections) hasConnection(pc transport.PhysicalConn, address string) bool {
	for _, connection := range c.slots {
		if connection != nil && (connection.conn == pc || connection.node.GetAddress() == address) {
			return true
		}
	}
	return false
}

func tokenFatal(err error) bool {
	var local *attachmentError
	if errors.As(err, &local) {
		return true
	}
	var rpcError twirp.Error
	if errors.As(err, &rpcError) {
		switch rpcError.Code() {
		case twirp.InvalidArgument, twirp.Unauthenticated, twirp.PermissionDenied, twirp.NotFound:
			return true
		}
	}
	return false
}
