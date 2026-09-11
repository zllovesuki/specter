package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"go.uber.org/zap"
)

// TunnelSyncResult reports the last publication acknowledgement for a configured
// tunnel. Publication is not an end-to-end health check of its target.
type TunnelSyncResult struct {
	Hostname           string `json:"hostname"`
	Target             string `json:"target"`
	Published          bool   `json:"published"`
	PublishedEndpoints int    `json:"publishedEndpoints"`
	Error              string `json:"error,omitempty"`
}

type SyncResult struct {
	Applied     bool               `json:"applied"`
	Saved       bool               `json:"saved"`
	Error       string             `json:"error,omitempty"`
	AttemptedAt *time.Time         `json:"attemptedAt,omitempty"`
	Tunnels     []TunnelSyncResult `json:"tunnels"`
}

func (r SyncResult) pendingPublication() bool {
	for _, tunnel := range r.Tunnels {
		if tunnel.Error != "" {
			return true
		}
	}
	return false
}

type publicationState struct {
	endpoints string
	published int
}

// ConfigSaveError means that the operation changed live state, but the updated
// configuration could not be saved. Reloading the old file may undo that change.
type ConfigSaveError struct{ Err error }

func (e *ConfigSaveError) Error() string {
	return fmt.Sprintf("change applied, but configuration was not saved: %v", e.Err)
}

func (e *ConfigSaveError) Unwrap() error { return e.Err }

func (c *Client) requestHostname(ctx context.Context) (string, error) {
	connected := c.getConnectedNodes()
	if len(connected) == 0 {
		return "", fmt.Errorf("no rpc candidates available")
	}
	// Generation is not idempotent. After an ambiguous failure, the next sync
	// queries registered hostnames before attempting to generate another one.
	callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	candidate := connected[c.hostnameNextCandidate%uint64(len(connected))]
	resp, err := c.tunnelClient.GenerateHostname(rpc.WithNode(callCtx, candidate), &protocol.GenerateHostnameRequest{})
	if err != nil {
		c.hostnameNextCandidate++
		return "", err
	}
	if resp.GetHostname() == "" {
		c.hostnameNextCandidate++
		return "", fmt.Errorf("server returned an empty generated hostname")
	}
	return resp.GetHostname(), nil
}

func endpointSignature(nodes []*protocol.Node) string {
	var signature strings.Builder
	for _, node := range nodes {
		fmt.Fprintf(&signature, "%d:%s;", node.GetId(), node.GetAddress())
	}
	return signature.String()
}

// SyncConfigTunnels explicitly republishes the current configuration. Automatic
// retries use the successful acknowledgements from this attempt to retry only
// unfinished work.
func (c *Client) SyncConfigTunnels(ctx context.Context) SyncResult {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	return c.syncConfigTunnels(ctx, true)
}

// syncConfigTunnels requires syncMu to serialize reload, removal, and retries.
func (c *Client) syncConfigTunnels(ctx context.Context, force bool) SyncResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if force || c.publication == nil {
		c.publication = make(map[string]publicationState)
	}
	c.configMu.RLock()
	tunnels := append([]Tunnel{}, c.Configuration.Tunnels...)
	c.configMu.RUnlock()

	now := time.Now()
	result := SyncResult{Applied: true, AttemptedAt: &now, Tunnels: make([]TunnelSyncResult, len(tunnels))}
	var syncErrors []error
	for i, tunnel := range tunnels {
		result.Tunnels[i] = TunnelSyncResult{Hostname: tunnel.Hostname, Target: tunnel.Target}
	}

	c.Logger.Info("Synchronizing tunnels in config file with specter", zap.Int("tunnels", len(tunnels)))

	c.syncStateMu.RLock()
	result.Saved = !c.lastSync.Applied || c.lastSync.Saved
	c.syncStateMu.RUnlock()
	hostnameAssigned := false
	needsHostname := false
	for _, tunnel := range tunnels {
		needsHostname = needsHostname || tunnel.Hostname == ""
	}
	available := make([]string, 0)
	var lookupErr error
	if force || needsHostname {
		registered, err := c.GetRegisteredHostnames(ctx)
		if err != nil {
			lookupErr = fmt.Errorf("querying registered hostnames: %w", err)
			syncErrors = append(syncErrors, lookupErr)
		} else {
			inUse := make(map[string]bool)
			for _, tunnel := range tunnels {
				inUse[tunnel.Hostname] = true
			}
			for _, hostname := range registered {
				if !strings.Contains(hostname, ".") && !inUse[hostname] {
					available = append(available, hostname)
					inUse[hostname] = true
				}
			}
		}
	}

	connected := c.getConnectedNodes()
	endpoints := endpointSignature(connected)
	var generationErr error
	for i := range tunnels {
		tunnel := &tunnels[i]
		outcome := &result.Tunnels[i]
		if tunnel.Hostname == "" {
			if lookupErr != nil {
				outcome.Error = lookupErr.Error()
				continue
			}
			var err error
			if len(available) > 0 {
				tunnel.Hostname, available = available[0], available[1:]
			} else if generationErr != nil {
				err = fmt.Errorf("hostname generation deferred until registered names are reconciled: %w", generationErr)
			} else {
				tunnel.Hostname, err = c.requestHostname(ctx)
				generationErr = err
			}
			if err != nil {
				outcome.Error = fmt.Sprintf("requesting hostname: %v", err)
				syncErrors = append(syncErrors, fmt.Errorf("%s: %s", tunnel.Target, outcome.Error))
				continue
			}
			outcome.Hostname = tunnel.Hostname
			hostnameAssigned = true
		}

		if previous, ok := c.publication[tunnel.Hostname]; !force && ok && previous.endpoints == endpoints {
			outcome.Published = true
			outcome.PublishedEndpoints = previous.published
			continue
		}
		// A fresh attempt can partially replace the numbered routing slots.
		// Its failure invalidates any earlier acknowledgement, even if a later
		// retry returns to the same gateway ordering as that acknowledgement.
		delete(c.publication, tunnel.Hostname)
		published, err := c.publishTunnel(ctx, tunnel.Hostname, connected)
		outcome.PublishedEndpoints = len(published)
		outcome.Published = len(published) > 0
		if err == nil && len(published) < len(connected) {
			err = fmt.Errorf("published %d of %d connected gateways", len(published), len(connected))
		}
		if err == nil && len(published) == 0 {
			err = fmt.Errorf("no gateways acknowledged publication")
		}
		if err != nil {
			outcome.Error = err.Error()
			syncErrors = append(syncErrors, fmt.Errorf("%s: %w", tunnel.Hostname, err))
			c.Logger.Error("Failed to fully publish tunnel", zap.String("hostname", tunnel.Hostname), zap.Error(err))
			continue
		}
		c.publication[tunnel.Hostname] = publicationState{endpoints: endpoints, published: len(published)}
		fqdn := tunnel.Hostname
		if !strings.Contains(fqdn, ".") {
			fqdn = fmt.Sprintf("%s.%s", fqdn, c.rootDomain.Load())
		}
		c.Logger.Info("Tunnel published", zap.String("hostname", fqdn), zap.String("target", tunnel.Target), zap.Int("published", len(published)))
	}

	// Retain generated hostnames in live state even when publication or saving
	// fails, so retrying cannot generate a second name for the same tunnel.
	// Publication-only retries must not overwrite edits waiting in the YAML
	// file for an explicit reload.
	if force || hostnameAssigned {
		err := c.RebuildTunnels(tunnels)
		result.Saved = err == nil
		syncErrors = append(syncErrors, err)
	} else if !result.Saved {
		syncErrors = append(syncErrors, errors.New("configuration changes are not saved"))
	}
	if err := errors.Join(syncErrors...); err != nil {
		result.Error = err.Error()
	}
	c.recordSyncResult(result)
	return result
}

func (c *Client) recordSyncResult(result SyncResult) {
	c.syncStateMu.Lock()
	defer c.syncStateMu.Unlock()
	c.lastSync = result
	c.lastSync.Tunnels = append([]TunnelSyncResult{}, result.Tunnels...)
	if !result.pendingPublication() {
		c.syncBackoff = 0
		c.nextSync = time.Time{}
		return
	}
	if c.syncBackoff == 0 {
		c.syncBackoff = checkInterval
	} else {
		c.syncBackoff *= 2
	}
	if c.syncBackoff > 5*time.Minute {
		c.syncBackoff = 5 * time.Minute
	}
	c.nextSync = time.Now().Add(c.syncBackoff)
}

func (c *Client) publishTunnel(ctx context.Context, hostname string, connected []*protocol.Node) ([]*protocol.Node, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.RegisteredHostnamesResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.RegisteredHostnames(ctx, &protocol.RegisteredHostnamesRequest{})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetHostnames(), nil
}

func (c *Client) RebuildTunnels(tunnels []Tunnel) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	next := *c.Configuration
	next.Tunnels = append([]Tunnel{}, tunnels...)
	if err := next.validate(); err != nil {
		return err
	}
	diff := diffTunnels(c.Configuration.Tunnels, next.Tunnels)
	c.closeOutdatedProxies(diff...)
	c.Configuration.Tunnels = next.Tunnels
	c.Configuration.buildRouter(diff...)
	if err := c.Configuration.writeFile(); err != nil {
		return &ConfigSaveError{Err: err}
	}
	return nil
}

func (c *Client) tunnelRemovalWrapper(tunnel Tunnel, fn func() error) error {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
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
	c.Configuration.validate()
	c.Configuration.buildRouter(tunnel)
	delete(c.publication, tunnel.Hostname)

	var saveErr error
	if err := c.Configuration.writeFile(); err != nil {
		saveErr = &ConfigSaveError{Err: err}
	}
	c.syncStateMu.RLock()
	result := c.lastSync
	c.syncStateMu.RUnlock()
	remaining := make([]TunnelSyncResult, 0, len(c.Configuration.Tunnels))
	var pendingErrors []error
	for _, tunnel := range c.Configuration.Tunnels {
		outcome := TunnelSyncResult{Hostname: tunnel.Hostname, Target: tunnel.Target, Error: "publication pending"}
		for _, previous := range result.Tunnels {
			if previous.Hostname == tunnel.Hostname && previous.Target == tunnel.Target {
				outcome = previous
				break
			}
		}
		remaining = append(remaining, outcome)
		if outcome.Error != "" {
			pendingErrors = append(pendingErrors, fmt.Errorf("%s: %s", tunnel.Hostname, outcome.Error))
		}
	}
	pendingErrors = append(pendingErrors, saveErr)
	result.Tunnels, result.Applied, result.Saved, result.Error = remaining, true, saveErr == nil, ""
	if err := errors.Join(pendingErrors...); err != nil {
		result.Error = err.Error()
	}
	c.recordSyncResult(result)
	return saveErr
}

func (c *Client) UnpublishTunnel(ctx context.Context, tunnel Tunnel) error {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
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
