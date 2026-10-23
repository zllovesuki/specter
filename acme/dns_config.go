package acme

import (
	"bytes"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/util"

	"github.com/miekg/dns"
)

const (
	DNSConfigName   = "dns"
	DNSConfigSchema = 1
)

// DNSConfig contains the static records for the delegated ACME challenge zone.
// ACME challenge TXT records remain owned by the distributed KV provider.
type DNSConfig struct {
	Zone        string          `json:"zone"`
	Nameservers []DNSNameserver `json:"nameservers"`
}

type DNSNameserver struct {
	Host      string   `json:"host"`
	Addresses []string `json:"addresses"`
}

// DNSConfigError identifies the input field that prevents a DNS configuration
// from being used. Paths use zero-based indexes into the submitted document.
type DNSConfigError struct {
	Field   string
	Message string
}

func (e *DNSConfigError) Error() string {
	return e.Field + ": " + e.Message
}

// DecodeDNSConfig rejects unsupported schemas and fields rather than silently
// interpreting a newer document as an older configuration. The generic store
// can still retain and replicate that document without understanding it.
func DecodeDNSConfig(record config.Record, localZone string) (DNSConfig, error) {
	if err := record.Validate(); err != nil {
		return DNSConfig{}, fmt.Errorf("invalid shared DNS record: %w", err)
	}
	if record.Name != DNSConfigName {
		return DNSConfig{}, fmt.Errorf("expected shared DNS configuration, got %q", record.Name)
	}
	if record.Version.Schema != DNSConfigSchema {
		return DNSConfig{}, fmt.Errorf("unsupported DNS configuration schema %d (supported: %d)", record.Version.Schema, DNSConfigSchema)
	}
	var dnsConfig DNSConfig
	if err := util.DecodeJSON(bytes.NewReader(record.Value), &dnsConfig); err != nil {
		return DNSConfig{}, fmt.Errorf("invalid DNS configuration: %w", err)
	}
	return ValidateDNSConfig(dnsConfig, localZone)
}

// ValidateDNSConfig returns a canonical copy. A nonempty localZone binds the
// document to this server's existing ACME zone; an initializer can leave it
// empty to validate a document before any server has been started.
func ValidateDNSConfig(dnsConfig DNSConfig, localZone string) (DNSConfig, error) {
	zone, err := dnsConfigHostname(dnsConfig.Zone)
	if err != nil {
		return DNSConfig{}, &DNSConfigError{Field: "zone", Message: err.Error()}
	}
	if localZone != "" {
		local, err := dnsConfigHostname(localZone)
		if err != nil {
			return DNSConfig{}, fmt.Errorf("invalid local ACME zone: %w", err)
		}
		if zone != local {
			return DNSConfig{}, &DNSConfigError{Field: "zone", Message: fmt.Sprintf("must match the local ACME zone %s", local)}
		}
	}
	if len(dnsConfig.Nameservers) == 0 {
		return DNSConfig{}, &DNSConfigError{Field: "nameservers", Message: "at least one nameserver is required"}
	}
	validated := DNSConfig{Zone: zone, Nameservers: make([]DNSNameserver, 0, len(dnsConfig.Nameservers))}
	hosts := make(map[string]struct{}, len(dnsConfig.Nameservers))
	for i, ns := range dnsConfig.Nameservers {
		field := fmt.Sprintf("nameservers.%d", i)
		host, err := dnsConfigHostname(ns.Host)
		if err != nil {
			return DNSConfig{}, &DNSConfigError{Field: field + ".host", Message: err.Error()}
		}
		if _, exists := hosts[host]; exists {
			return DNSConfig{}, &DNSConfigError{Field: field + ".host", Message: "nameserver is listed more than once"}
		}
		hosts[host] = struct{}{}
		if len(ns.Addresses) == 0 {
			return DNSConfig{}, &DNSConfigError{Field: field + ".addresses", Message: "at least one IP address is required"}
		}
		addresses := make([]string, 0, len(ns.Addresses))
		seen := make(map[netip.Addr]struct{}, len(ns.Addresses))
		for j, raw := range ns.Addresses {
			address, err := netip.ParseAddr(raw)
			if err != nil || address.Zone() != "" {
				return DNSConfig{}, &DNSConfigError{Field: fmt.Sprintf("%s.addresses.%d", field, j), Message: "must be an IPv4 or IPv6 address without a port or zone"}
			}
			address = address.Unmap()
			if _, exists := seen[address]; exists {
				return DNSConfig{}, &DNSConfigError{Field: fmt.Sprintf("%s.addresses.%d", field, j), Message: "address is listed more than once for this nameserver"}
			}
			seen[address] = struct{}{}
			addresses = append(addresses, address.String())
		}
		sort.Strings(addresses)
		validated.Nameservers = append(validated.Nameservers, DNSNameserver{Host: host, Addresses: addresses})
	}
	sort.Slice(validated.Nameservers, func(i, j int) bool {
		return validated.Nameservers[i].Host < validated.Nameservers[j].Host
	})
	return validated, nil
}

func dnsConfigHostname(value string) (string, error) {
	name := dns.CanonicalName(value)
	if len(name) > 254 || name == "." {
		return "", fmt.Errorf("must be a DNS hostname")
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("must contain DNS labels of 1 to 63 characters without leading or trailing hyphens")
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return "", fmt.Errorf("must contain only ASCII letters, digits, hyphens, and dots")
			}
		}
	}
	if _, err := netip.ParseAddr(strings.TrimSuffix(name, ".")); err == nil {
		return "", fmt.Errorf("must be a DNS hostname, not an IP address")
	}
	return name, nil
}
