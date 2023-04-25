package client

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/tun/client"
	"kon.nect.sh/specter/tun/client/dialer"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func cmdAcme(ctx *cli.Context) error {
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

	resp, err := c.GetAcmeInstruction(ctx.Context, hostname)
	if err != nil {
		return fmt.Errorf("failed to query acme instruction: %w", err)
	}

	outerTable := table.NewWriter()
	outerTable.SetOutputMirror(os.Stdout)

	outerTable.AppendHeader(table.Row{"\nPlease add the following DNS record to validate ownership:"})

	infoTable := table.NewWriter()
	infoTable.AppendHeader(table.Row{"Name", "Type", "Content"})
	infoTable.AppendRow(table.Row{resp.GetName(), "CNAME", resp.GetContent()})

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
