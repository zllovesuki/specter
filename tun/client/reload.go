package client

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (c *Client) reloadOnSignal(ctx context.Context) {
	defer c.closeWg.Done()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ctx.Done():
			return
		case <-c.ReloadSignal:
			c.Logger.Info("Received SIGHUP, reloading config")
			if result := c.doReload(ctx); result.Error != "" {
				c.Logger.Warn("Configuration reload did not fully succeed", zap.String("error", result.Error))
			}
		}
	}
}

func (c *Client) doReload(ctx context.Context) SyncResult {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	onReload := func(prev, curr []Tunnel) {
		diff := diffTunnels(prev, curr)
		c.closeOutdatedProxies(diff...)
		c.Configuration.buildRouter(diff...)
	}
	c.configMu.Lock()
	if err := c.Configuration.reloadFile(onReload); err != nil {
		c.Logger.Error("Error reloading config file", zap.Error(err))
		c.configMu.Unlock()
		now := time.Now()
		return SyncResult{Error: err.Error(), AttemptedAt: &now, Tunnels: []TunnelSyncResult{}}
	}
	c.configMu.Unlock()
	return c.syncConfigTunnels(ctx, true)
}

func (c *Client) UpdateApex(apex string) {
	c.configMu.Lock()
	c.Configuration.Apex = apex
	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving to config file", zap.Error(err))
	}
	c.configMu.Unlock()
}

func (c *Client) GetCurrentConfig() *Config {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	return c.Configuration.clone()
}
