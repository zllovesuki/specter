package client

import (
	"encoding/json"
	"io"
	"time"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/miekg/dns"
)

type listTunnelItem struct {
	Hostname   string `json:"hostname"`
	Target     string `json:"target"`
	HeaderHost string `json:"headerHost"`
}

func (c *Client) FormatList(hostnames []string, output io.Writer) {
	items := make([]listTunnelItem, 0)

	tunnelMap := make(map[string]Tunnel)
	curr := c.GetCurrentConfig()
	for _, t := range curr.Tunnels {
		tunnelMap[t.Hostname] = t
	}

	for _, h := range hostnames {
		item := listTunnelItem{
			Hostname: h,
		}
		t, ok := tunnelMap[h]
		if ok {
			item.Target = t.Target
			item.HeaderHost = t.ProxyHeaderHost
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

type domainToken struct {
	ID         string `json:"id"`
	Hostname   string `json:"hostname"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
	Incomplete bool   `json:"incomplete"`
}

func formatDomainToken(grant *protocol.DelegationGrant) domainToken {
	item := domainToken{
		ID:         grant.GetId(),
		Hostname:   grant.GetHostname(),
		Incomplete: grant.GetIncomplete(),
	}
	if grant.GetExpiresAt() != 0 {
		item.ExpiresAt = time.Unix(grant.GetExpiresAt(), 0).UTC().Format(time.RFC3339)
	}
	return item
}

func formatDelegationJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func (c *Client) FormatDelegations(resp *protocol.ListDelegationsResponse, output io.Writer) error {
	items := make([]domainToken, 0, len(resp.GetGrants()))
	for _, grant := range resp.GetGrants() {
		items = append(items, formatDomainToken(grant))
	}
	return formatDelegationJSON(output, items)
}

func (c *Client) FormatMintedDelegation(resp *protocol.MintDelegationResponse, output io.Writer) error {
	return formatDelegationJSON(output, struct {
		Grant domainToken `json:"grant"`
		Token string      `json:"token"`
	}{
		Grant: formatDomainToken(resp.GetGrant()),
		Token: resp.GetToken(),
	})
}

func (c *Client) FormatRevokedDelegation(resp *protocol.RevokeDelegationResponse, output io.Writer) error {
	return formatDelegationJSON(output, struct {
		Revoked    bool   `json:"revoked"`
		IndexError string `json:"indexError,omitempty"`
	}{
		Revoked:    resp.GetRevoked(),
		IndexError: resp.GetIndexError(),
	})
}
