package client

import (
	"encoding/json"
	"io"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/miekg/dns"
)

type listTunnelItem struct {
	Hostname string `json:"hostname"`
	Target   string `json:"target"`
}

func (c *Client) FormatList(hostnames []string, output io.Writer) {
	items := make([]listTunnelItem, 0)

	tunnelMap := make(map[string]string)
	curr := c.GetCurrentConfig()
	for _, t := range curr.Tunnels {
		tunnelMap[t.Hostname] = t.Target
	}

	for _, h := range hostnames {
		item := listTunnelItem{
			Hostname: h,
		}
		target, ok := tunnelMap[h]
		if ok {
			item.Target = target
		} else {
			item.Target = "(unused)"
		}
		items = append(items, item)
	}

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.Encode(&items)
}

type acmeItem struct {
	Message string `json:"message"`
	Record  string `json:"record"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

func (c *Client) FormatAcme(resp *protocol.InstructionResponse, output io.Writer) {
	item := acmeItem{
		Message: "Please add the following DNS record to validate ownership",
		Record:  resp.GetName(),
		Type:    "CNAME",
		Content: resp.GetContent(),
	}

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.Encode(&item)
}

func (c *Client) FormatValidate(hostname string, resp *protocol.ValidateResponse, output io.Writer) {
	item := acmeItem{
		Message: "Ownership validated! Please add the following DNS record to be used with specter",
		Record:  dns.Fqdn(hostname),
		Type:    "CNAME",
		Content: resp.GetApex(),
	}

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.Encode(&item)
}
