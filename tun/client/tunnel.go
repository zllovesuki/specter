package client

import (
	"context"
	"fmt"
	"strings"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"go.uber.org/zap"
)

func (c *Client) requestHostname(ctx context.Context) (string, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.GenerateHostnameResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.GenerateHostname(ctx, &protocol.GenerateHostnameRequest{})
	})
	if err != nil {
		return "", err
	}
	return resp.GetHostname(), nil
}

func (c *Client) SyncConfigTunnels(ctx context.Context) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()

	c.configMu.RLock()
	tunnels := append([]Tunnel{}, c.Configuration.Tunnels...)
	c.configMu.RUnlock()

	c.Logger.Info("Synchronizing tunnels in config file with specter", zap.Int("tunnels", len(tunnels)))

	// reuse already assigned hostnames if possible
	registered, err := c.GetRegisteredHostnames(ctx)
	if err != nil {
		c.Logger.Error("Failed to query available hostnames", zap.Error(err))
		return
	}
	available := make([]string, 0)
	inused := make(map[string]string)
	for _, t := range tunnels {
		inused[t.Hostname] = t.Target
	}
	for _, hostname := range registered {
		// while we want to reuse hostnames, we want to reuse auto-generated hostnames only
		// so we don't accidentally point, say, pointing bastion.customdomain.com to MySQL
		if strings.Contains(hostname, ".") {
			continue
		}
		// filter out hostnames currently in used
		if _, ok := inused[hostname]; ok {
			continue
		}
		available = append(available, hostname)
	}

	// now assign a hostname to a target if they don't have one, either a new hostname or reused
	var name string
	for i, tunnel := range tunnels {
		if tunnel.Target == "" {
			continue
		}
		if tunnel.Hostname == "" {
			if len(available) > 0 {
				name, available = available[0], available[1:]
			} else {
				name, err = c.requestHostname(ctx)
				if err != nil {
					c.Logger.Error("Failed to request new hostname", zap.String("target", tunnel.Target), zap.Error(err))
					continue
				}
			}
			tunnels[i].Hostname = name
		}
		// TODO: assert that the hostname was assigned
	}

	connected := c.getConnectedNodes()
	apex := c.rootDomain.Load()

	for _, tunnel := range tunnels {
		if tunnel.Hostname == "" {
			continue
		}
		published, err := c.publishTunnel(ctx, tunnel.Hostname, connected)
		if err != nil {
			c.Logger.Error("Failed to publish tunnel", zap.String("hostname", tunnel.Hostname), zap.String("target", tunnel.Target), zap.Int("endpoints", len(connected)), zap.Error(err))
			continue
		}

		var fqdn string
		if strings.Contains(tunnel.Hostname, ".") {
			fqdn = tunnel.Hostname
		} else {
			fqdn = fmt.Sprintf("%s.%s", tunnel.Hostname, apex)
		}
		c.Logger.Info("Tunnel published", zap.String("hostname", fqdn), zap.String("target", tunnel.Target), zap.Int("published", len(published)))
	}

	c.RebuildTunnels(tunnels)
}

func (c *Client) publishTunnel(ctx context.Context, hostname string, connected []*protocol.Node) ([]*protocol.Node, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.PublishTunnelResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.PublishTunnel(ctx, &protocol.PublishTunnelRequest{
			Hostname: hostname,
			Servers:  connected,
		})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetPublished(), nil
}

func (c *Client) GetRegisteredHostnames(ctx context.Context) ([]string, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.RegisteredHostnamesResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.RegisteredHostnames(ctx, &protocol.RegisteredHostnamesRequest{})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetHostnames(), nil
}

func (c *Client) RebuildTunnels(tunnels []Tunnel) {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	diff := diffTunnels(c.Configuration.Tunnels, tunnels)
	c.closeOutdatedProxies(diff...)

	c.Configuration.Tunnels = tunnels
	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving to config file", zap.Error(err))
	}

	c.Configuration.validate()
	c.Configuration.buildRouter(diff...)
}

func (c *Client) tunnelRemovalWrapper(tunnel Tunnel, fn func() error) error {
	if err := fn(); err != nil {
		return err
	}

	c.configMu.Lock()
	defer c.configMu.Unlock()

	var index int = -1
	for i, t := range c.Configuration.Tunnels {
		if t.Hostname == tunnel.Hostname {
			index = i
			break
		}
	}
	if index == -1 {
		return nil
	}

	c.closeOutdatedProxies(tunnel)

	c.Configuration.Tunnels = append(c.Configuration.Tunnels[:index], c.Configuration.Tunnels[index+1:]...)
	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving to config file", zap.Error(err))
	}

	c.Configuration.validate()
	c.Configuration.buildRouter(tunnel)

	return nil
}

func (c *Client) UnpublishTunnel(ctx context.Context, tunnel Tunnel) error {
	err := c.tunnelRemovalWrapper(tunnel, func() error {
		_, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.UnpublishTunnelResponse, error) {
			ctx = rpc.WithNode(ctx, node)
			return c.tunnelClient.UnpublishTunnel(ctx, &protocol.UnpublishTunnelRequest{
				Hostname: tunnel.Hostname,
			})
		})
		return err
	})
	return err
}

func (c *Client) ReleaseTunnel(ctx context.Context, tunnel Tunnel) error {
	return c.tunnelRemovalWrapper(tunnel, func() error {
		_, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.ReleaseTunnelResponse, error) {
			ctx = rpc.WithNode(ctx, node)
			return c.tunnelClient.ReleaseTunnel(ctx, &protocol.ReleaseTunnelRequest{
				Hostname: tunnel.Hostname,
			})
		})
		return err
	})
}

func (c *Client) closeOutdatedProxies(tunnels ...Tunnel) {
	for _, t := range tunnels {
		proxy, loaded := c.proxies.LoadAndDelete(t.Hostname)
		if loaded {
			c.Logger.Info("Shutting down proxy", zap.String("hostname", t.Hostname), zap.String("target", t.Target))
			proxy.acceptor.Close()
			proxy.forwarder.Close()
		}
	}
}

func diffTunnels(old, new []Tunnel) []Tunnel {
	diff := make([]Tunnel, 0)
	oldMap := map[string]Tunnel{}
	newMap := map[string]Tunnel{}
	for _, o := range old {
		if o.Hostname == "" {
			continue
		}
		oldMap[o.Hostname] = o
	}
	for _, n := range new {
		if n.Hostname == "" {
			continue
		}
		newMap[n.Hostname] = n
	}
	// if new != old
	for hostname, tunnel := range newMap {
		oldTunnel, ok := oldMap[hostname]
		if ok && (oldTunnel.Target != tunnel.Target ||
			oldTunnel.Insecure != tunnel.Insecure ||
			oldTunnel.ProxyHeaderTimeout != tunnel.ProxyHeaderTimeout ||
			oldTunnel.ProxyHeaderHost != tunnel.ProxyHeaderHost ||
			oldTunnel.ProxyHeaderMode != tunnel.ProxyHeaderMode) {
			diff = append(diff, oldTunnel)
		}
	}
	// if old is gone
	for hostname, tunnel := range oldMap {
		if _, ok := newMap[hostname]; !ok {
			diff = append(diff, tunnel)
		}
	}
	return diff
}
