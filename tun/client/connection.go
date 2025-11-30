package client

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/spec/tun"

	"github.com/avast/retry-go/v4"
	"go.uber.org/zap"
)

func (c *Client) bootstrap(ctx context.Context, apex string) error {
	return c.openRPC(ctx, &protocol.Node{
		Address: apex,
	})
}

func (c *Client) openRPC(ctx context.Context, node *protocol.Node) error {
	if _, ok := c.connections.Load(node.GetAddress()); ok {
		return nil
	}

	callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	resp, err := c.ping(callCtx, node)
	if err != nil {
		return err
	}

	identity := resp.GetNode()
	c.Logger.Info("Connected to specter server", zap.String("addr", identity.GetAddress()))
	c.connections.Store(identity.GetAddress(), identity)

	return nil
}

func retryRPC[V any](c *Client, ctx context.Context, fn func(node *protocol.Node) (V, error)) (resp V, err error) {
	candidates := c.getConnectedNodes()
	err = retry.Do(func() error {
		var (
			candidate *protocol.Node
			rpcError  error
		)
		if len(candidates) > 0 {
			candidate, candidates = candidates[0], candidates[1:]
		}
		if candidate == nil {
			return fmt.Errorf("no rpc candidates available")
		}
		resp, rpcError = fn(candidate)
		return chord.ErrorMapper(rpcError)
	},
		retry.Context(ctx),
		retry.Attempts(2),
		retry.LastErrorOnly(true),
		retry.Delay(time.Millisecond*500),
		retry.RetryIf(chord.ErrorIsRetryable),
	)
	return
}

func (c *Client) getConnectedNodes() (nodes []*protocol.Node) {
	c.connections.Range(func(_ string, node *protocol.Node) bool {
		if len(nodes) < tun.NumRedundantLinks {
			nodes = append(nodes, node)
		}
		return true
	})

	// fast path exit if we don't have rtt enabled
	if c.Recorder == nil {
		return
	}

	// sort routes based on rtt to different gateways, so hostname/1 and rpc calls
	// always resolves to the gateway with the lowest rtt to the client
	rttLookup := make(map[string]time.Duration)
	for _, n := range nodes {
		m := c.Recorder.Snapshot(rtt.MakeMeasurementKey(n), time.Second*10)
		if m == nil {
			continue
		}
		rttLookup[rtt.MakeMeasurementKey(n)] = m.Average
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		l, lOK := rttLookup[rtt.MakeMeasurementKey(nodes[i])]
		r, rOK := rttLookup[rtt.MakeMeasurementKey(nodes[j])]
		if lOK && !rOK {
			return true
		}
		if !lOK && rOK {
			return false
		}
		return l < r
	})

	c.Logger.Debug("rtt information", zap.String("table", fmt.Sprint(rttLookup)))

	return nodes
}

func (c *Client) getAliveNodes(ctx context.Context) (alive []*protocol.Node, dead int) {
	alive = make([]*protocol.Node, 0)
	c.connections.Range(func(addr string, node *protocol.Node) bool {
		func() {
			callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()

			_, err := c.ping(callCtx, node)
			if err != nil {
				c.connections.Delete(addr)
				dead++
			} else {
				alive = append(alive, node)
			}
		}()
		return true
	})
	return
}

func (c *Client) reBootstrap(ctx context.Context) {
	defer c.closeWg.Done()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			connected := c.getConnectedNodes()
			if len(connected) > 0 {
				continue
			}
			c.Logger.Info("No connected nodes, re-bootstrapping using apex")
			c.configMu.RLock()
			apex := c.ClientConfig.Configuration.Apex
			c.configMu.RUnlock()
			if err := c.bootstrap(ctx, apex); err != nil {
				c.Logger.Error("Failed to rebootstrap connection to specter", zap.Error(err))
			}
		}
	}
}

func (c *Client) periodicReconnection(ctx context.Context) {
	defer c.closeWg.Done()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			prev, failed := c.getAliveNodes(ctx)
			if failed > 0 {
				c.Logger.Info("Some connections have failed, opening more connections to specter server", zap.Int("dead", failed))
			}
			if err := c.maintainConnections(ctx); err != nil {
				continue
			}
			now, _ := c.getAliveNodes(ctx)

			var seed uint64 = 0
			pH := c.hash(seed, prev)
			nH := c.hash(seed, now)
			c.Logger.Debug("Alive nodes delta", zap.Int("prevNum", len(prev)), zap.Uint64("prevHash", pH), zap.Int("currNum", len(now)), zap.Uint64("currHash", nH))

			if failed > 0 || pH != nH {
				c.Logger.Info("Connections with specter server have changed", zap.Int("previous", len(prev)), zap.Int("current", len(now)))
				c.SyncConfigTunnels(ctx)
			}
		}
	}
}

func (c *Client) maintainConnections(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	nodes, err := c.requestCandidates(callCtx)
	if err != nil {
		return err
	}
	c.Logger.Debug("Candidates for RPC connections", zap.Int("num", len(nodes)))

	for _, node := range nodes {
		if c.connections.Len() >= tun.NumRedundantLinks {
			return nil
		}
		if err := c.openRPC(ctx, node); err != nil {
			return fmt.Errorf("connecting to specter server: %w", err)
		}
	}

	return nil
}

func (c *Client) ping(ctx context.Context, node *protocol.Node) (*protocol.ClientPingResponse, error) {
	return c.tunnelClient.Ping(rpc.WithNode(ctx, node), &protocol.ClientPingRequest{})
}

func (c *Client) requestCandidates(ctx context.Context) ([]*protocol.Node, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.GetNodesResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.GetNodes(ctx, &protocol.GetNodesRequest{})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetNodes(), nil
}
