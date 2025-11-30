package client

import (
	"context"

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
			c.doReload(ctx)
		}
	}
}

func (c *Client) doReload(ctx context.Context) {
	onReload := func(prev, curr []Tunnel) {
		diff := diffTunnels(prev, curr)
		c.closeOutdatedProxies(diff...)
		c.Configuration.buildRouter(diff...)
	}
	c.configMu.Lock()
	if err := c.Configuration.reloadFile(onReload); err != nil {
		c.Logger.Error("Error reloading config file", zap.Error(err))
		c.configMu.Unlock()
		return
	}
	c.configMu.Unlock()
	c.SyncConfigTunnels(ctx)
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
