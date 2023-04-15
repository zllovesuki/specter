package client

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/tun/client"
	"kon.nect.sh/specter/tun/client/dialer"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/miekg/dns"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func cmdValidate(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	hostname := ctx.Args().First()
	if hostname == "" {
		return fmt.Errorf("missing hostname in argument")
	}

	cfg, err := client.NewConfig(ctx.String("config"))
	if err != nil {
		return err
	}

	parsed, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return err
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	_, transport := createTransport(ctx, logger, cfg, parsed, nil)
	defer transport.Stop()

	c, err := client.NewClient(ctx.Context, client.ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		ServerTransport: transport,
		ReloadSignal:    s,
	})
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Register(ctx.Context); err != nil {
		return err
	}

	resp, err := c.RequestAcmeValidation(ctx.Context, hostname)
	if err != nil {
		return fmt.Errorf("failed to validate acme setup: %w", err)
	}

	outerTable := table.NewWriter()
	outerTable.SetOutputMirror(os.Stdout)

	outerTable.AppendHeader(table.Row{"\nOwnership validated! Please add the following DNS record to be used with specter:"})

	infoTable := table.NewWriter()
	infoTable.AppendHeader(table.Row{"Name", "Type", "Content"})
	infoTable.AppendRow(table.Row{dns.Fqdn(hostname), "CNAME", dns.Fqdn(resp.GetApex())})

	infoTable.SetStyle(table.StyleDefault)
	infoTable.Style().Options.SeparateRows = true
	info := infoTable.Render()

	outerTable.AppendRow(table.Row{info})
	outerTable.SetStyle(table.StyleDefault)
	outerTable.Style().Options.DrawBorder = false
	outerTable.Style().Options.SeparateHeader = false
	outerTable.Style().Options.SeparateColumns = false
	outerTable.Style().Options.SeparateRows = false
	outerTable.Render()

	return nil
}
