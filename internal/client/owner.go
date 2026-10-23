package client

import (
	"context"

	"go.miragespace.co/specter/tun/client"
)

// OpenOwner opens a registered owner for management commands. It cannot obtain
// or renew owner certificates; the tunnel runtime owns those operations.
func OpenOwner(ctx context.Context, configPath string, opts Options) (*client.Client, func(), error) {
	cfg, apex, err := loadOwnerConfig(configPath)
	if err != nil {
		return nil, nil, err
	}
	_, transport, closeTransport, err := openTransport(opts, apex, nil)
	if err != nil {
		return nil, nil, err
	}
	c, err := client.NewClient(ctx, client.ClientConfig{
		Logger:          opts.Logger,
		Configuration:   cfg,
		ServerTransport: transport,
	})
	if err != nil {
		closeTransport()
		return nil, nil, err
	}
	closeClient := func() {
		c.Close()
		closeTransport()
	}
	if err := c.Register(ctx); err != nil {
		closeClient()
		return nil, nil, err
	}
	return c, closeClient, nil
}
