package client

import (
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/miekg/dns"
	"kon.nect.sh/specter/spec/protocol"
)

func (c *Client) FormatList(hostnames []string, output io.Writer) {
	tunnelTable := table.NewWriter()
	tunnelTable.SetOutputMirror(output)

	tunnelMap := make(map[string]string)
	curr := c.GetCurrentConfig()
	for _, t := range curr.Tunnels {
		tunnelMap[t.Hostname] = t.Target
	}

	tunnelTable.AppendHeader(table.Row{"Hostname", "Target"})
	for _, h := range hostnames {
		target, ok := tunnelMap[h]
		if ok {
			tunnelTable.AppendRow(table.Row{h, target})
		} else {
			tunnelTable.AppendRow(table.Row{h, "(unused)"})
		}
	}

	tunnelTable.SetStyle(table.StyleDefault)
	tunnelTable.Style().Options.SeparateRows = true
	tunnelTable.Render()
}

func (c *Client) FormatAcme(resp *protocol.InstructionResponse, output io.Writer) {
	outerTable := table.NewWriter()
	outerTable.SetOutputMirror(output)

	outerTable.AppendHeader(table.Row{"Please add the following DNS record to validate ownership:"})

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
}

func (c *Client) FormatValidate(hostname string, resp *protocol.ValidateResponse, output io.Writer) {
	outerTable := table.NewWriter()
	outerTable.SetOutputMirror(output)

	outerTable.AppendHeader(table.Row{"Ownership validated! Please add the following DNS record to be used with specter:"})

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
}
