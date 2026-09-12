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

	"github.com/avast/retry-go/v5"
	"go.uber.org/zap"
)

func (c *Client) bootstrap(ctx context.Context, apex string) error {
	c.Logger.Info("Bootstraping connection to specter server", zap.String("addr", apex))
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
	retrier := retry.New(
		retry.Context(ctx),
		retry.Attempts(2),
		retry.LastErrorOnly(true),
		retry.Delay(time.Millisecond*500),
		retry.RetryIf(chord.ErrorIsRetryable),
	)
	err = retrier.Do(func() error {
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
	})
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

func (c *Client) periodicReconnection(ctx context.Context) {
	defer c.closeWg.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-c.closeCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	runReconnectLoop(ctx, nil, func(ctx context.Context) (time.Duration, error) {
		c.reconcileConnections(ctx)
		return 0, nil
	})
}

// runReconnectLoop serializes maintenance and preserves the full client's ticker
// cadence. A positive retry delay takes precedence over ticks and wakeups until
// it fires, avoiding duplicate attempts during a complete outage.
func runReconnectLoop(ctx context.Context, wake <-chan struct{}, reconcile func(context.Context) (time.Duration, error)) error {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	retry := time.NewTimer(checkInterval)
	retry.Stop()
	defer retry.Stop()
	var retryC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if retryC != nil {
				continue
			}
		case <-wake:
			if retryC != nil {
				continue
			}
		case <-retryC:
			retryC = nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		delay, err := reconcile(ctx)
		if err != nil {
			return err
		}
		if delay > 0 {
			retry.Reset(delay)
			retryC = retry.C
		}
	}
}

func (c *Client) reconcileConnections(ctx context.Context) {
	prev, failed := c.getAliveNodes(ctx)
	if failed > 0 {
		c.Logger.Info("Some connections have failed, opening more connections to specter server", zap.Int("dead", failed))
	}
	if err := c.maintainConnections(ctx); err != nil {
		// A candidate lookup can fail while existing gateways still work. It
		// must not prevent retrying previously failed tunnel publication.
		c.Logger.Warn("Failed to maintain gateway connections", zap.Error(err))
	}
	now, _ := c.getAliveNodes(ctx)
	changed := failed > 0 || endpointSignature(prev) != endpointSignature(now)

	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	c.syncStateMu.RLock()
	retryPending := c.lastSync.pendingPublication() && !time.Now().Before(c.nextSync)
	c.syncStateMu.RUnlock()
	if changed {
		clear(c.publication)
	}
	if changed || retryPending {
		c.syncConfigTunnels(ctx, false)
	}
}

func (c *Client) maintainConnections(ctx context.Context) error {
	if c.connections.Len() == 0 {
		c.Logger.Info("No connected nodes, re-bootstrapping using apex")
		c.configMu.RLock()
		apex := c.ClientConfig.Configuration.Apex
		c.configMu.RUnlock()
		if err := c.bootstrap(ctx, apex); err != nil {
			return fmt.Errorf("rebootstrapping connection to specter: %w", err)
		}
	}

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
