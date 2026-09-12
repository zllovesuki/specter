package client

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func tokenCommand() *cli.Command {
	config := func() cli.Flag {
		return &cli.StringFlag{
			Name:     "config",
			Aliases:  []string{"c"},
			Required: true,
			Usage:    "owner configuration file; stop the owner client before using these commands",
		}
	}
	return &cli.Command{
		Name:  "token",
		Usage: "Manage domain tokens with a stopped owner client",
		Commands: []*cli.Command{
			{
				Name:      "mint",
				Usage:     "Mint a bearer token for a registered hostname",
				ArgsUsage: "<hostname>",
				Flags: []cli.Flag{config(), &cli.StringFlag{
					Name:  "expires-at",
					Usage: "optional expiry in RFC3339 format (default: no expiry)",
				}},
				Action: cmdToken,
			},
			{
				Name:      "list",
				Usage:     "List grants, including incomplete grants",
				ArgsUsage: " ",
				Flags:     []cli.Flag{config()},
				Action:    cmdToken,
			},
			{
				Name:      "revoke",
				Usage:     "Revoke a grant by its ID",
				ArgsUsage: "<id>",
				Flags:     []cli.Flag{config()},
				Action:    cmdToken,
			},
		},
	}
}

func cmdToken(ctx context.Context, cmd *cli.Command) error {
	expected := 1
	if cmd.Name == "list" {
		expected = 0
	}
	if cmd.Args().Len() != expected {
		return fmt.Errorf("token %s expects %d argument(s)", cmd.Name, expected)
	}
	var expiry time.Time
	if cmd.String("expires-at") != "" {
		parsed, err := time.Parse(time.RFC3339, cmd.String("expires-at"))
		if err != nil {
			return fmt.Errorf("expires-at must be RFC3339: %w", err)
		}
		expiry = parsed
	}
	cfg, err := client.NewConfig(cmd.String("config"))
	if err != nil {
		return err
	}
	apex, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return err
	}
	listener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	defer listener.Close()
	quicTransport := &quic.Transport{Conn: listener}
	defer quicTransport.Close()
	logger := cmd.Root().Metadata["logger"].(*zap.Logger)
	_, tp := createTransport(cmd, transportCfg{
		logger: logger,
		quicTp: quicTransport,
		apex:   apex,
	})
	defer tp.Stop()
	c, err := client.NewClient(ctx, client.ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		ServerTransport: tp,
	})
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Register(ctx); err != nil {
		return err
	}
	switch cmd.Name {
	case "mint":
		resp, err := c.MintDelegation(ctx, cmd.Args().First(), expiry)
		if err != nil {
			return err
		}
		return c.FormatMintedDelegation(resp, cmd.Root().Writer)
	case "list":
		resp, err := c.ListDelegations(ctx)
		if err != nil {
			return err
		}
		return c.FormatDelegations(resp, cmd.Root().Writer)
	case "revoke":
		resp, err := c.RevokeDelegation(ctx, cmd.Args().First())
		if err != nil {
			return err
		}
		return c.FormatRevokedDelegation(resp, cmd.Root().Writer)
	}
	return nil
}
