package acme

import (
	"encoding/json"
	"strings"
	"testing"

	"go.miragespace.co/specter/config"

	"github.com/stretchr/testify/require"
)

func dnsTestConfig() DNSConfig {
	return DNSConfig{Zone: testDomain, Nameservers: []DNSNameserver{{Host: testNSName, Addresses: []string{testNSContentA, testNSContentAAAA}}}}
}

func dnsTestRecord(t *testing.T, revision uint64, dnsConfig DNSConfig) config.Record {
	t.Helper()
	value, err := json.Marshal(dnsConfig)
	require.NoError(t, err)
	record, err := config.NewRecord(DNSConfigName, DNSConfigSchema, revision, value)
	require.NoError(t, err)
	return record
}

func TestDNSConfigCanonicalizesWithoutMutatingInput(t *testing.T) {
	input := DNSConfig{
		Zone: "ACME.Example.Com",
		Nameservers: []DNSNameserver{
			{Host: "NS2.ACME.Example.Com", Addresses: []string{"::ffff:192.0.2.2", "2001:0db8::2"}},
			{Host: "NS1.EDGE.ACME.EXAMPLE.COM.", Addresses: []string{"192.0.2.1"}},
			{Host: "ns.external.example", Addresses: []string{"192.0.2.3"}},
		},
	}
	got, err := ValidateDNSConfig(input, "acme.example.com.")
	require.NoError(t, err)
	require.Equal(t, "acme.example.com.", got.Zone)
	require.Equal(t, []DNSNameserver{
		{Host: "ns.external.example.", Addresses: []string{"192.0.2.3"}},
		{Host: "ns1.edge.acme.example.com.", Addresses: []string{"192.0.2.1"}},
		{Host: "ns2.acme.example.com.", Addresses: []string{"192.0.2.2", "2001:db8::2"}},
	}, got.Nameservers)
	require.Equal(t, "ACME.Example.Com", input.Zone)
	require.Equal(t, "NS2.ACME.Example.Com", input.Nameservers[0].Host)
	require.Equal(t, "::ffff:192.0.2.2", input.Nameservers[0].Addresses[0])

	_, err = ValidateDNSConfig(input, "")
	require.NoError(t, err, "offline initialization does not need a local ACME zone")
}

func TestDNSConfigRejectsUnusableRecords(t *testing.T) {
	tests := []struct {
		name   string
		change func(*DNSConfig)
		field  string
	}{
		{"empty zone", func(c *DNSConfig) { c.Zone = "" }, "zone"},
		{"wrong zone", func(c *DNSConfig) { c.Zone = "other.example.com" }, "zone"},
		{"empty label", func(c *DNSConfig) { c.Zone = "acme..example.com" }, "zone"},
		{"long label", func(c *DNSConfig) { c.Zone = strings.Repeat("a", 64) + ".example.com" }, "zone"},
		{"long hostname", func(c *DNSConfig) { c.Zone = strings.Repeat(strings.Repeat("a", 63)+".", 4) }, "zone"},
		{"hyphen label", func(c *DNSConfig) { c.Nameservers[0].Host = "-ns.example.com" }, "nameservers.0.host"},
		{"trailing hyphen", func(c *DNSConfig) { c.Nameservers[0].Host = "ns-.example.com" }, "nameservers.0.host"},
		{"space", func(c *DNSConfig) { c.Nameservers[0].Host = "ns .example.com" }, "nameservers.0.host"},
		{"escaped label", func(c *DNSConfig) { c.Nameservers[0].Host = `ns\046.example.com` }, "nameservers.0.host"},
		{"ip host", func(c *DNSConfig) { c.Nameservers[0].Host = "192.0.2.1" }, "nameservers.0.host"},
		{"wildcard", func(c *DNSConfig) { c.Nameservers[0].Host = "*.example.com" }, "nameservers.0.host"},
		{"empty nameservers", func(c *DNSConfig) { c.Nameservers = nil }, "nameservers"},
		{"empty host", func(c *DNSConfig) { c.Nameservers[0].Host = "" }, "nameservers.0.host"},
		{"empty addresses", func(c *DNSConfig) { c.Nameservers[0].Addresses = nil }, "nameservers.0.addresses"},
		{"invalid IP", func(c *DNSConfig) { c.Nameservers[0].Addresses = []string{"192.0.2.256"} }, "nameservers.0.addresses.0"},
		{"hostname address", func(c *DNSConfig) { c.Nameservers[0].Addresses = []string{"dns.example.com"} }, "nameservers.0.addresses.0"},
		{"address port", func(c *DNSConfig) { c.Nameservers[0].Addresses = []string{"192.0.2.1:53"} }, "nameservers.0.addresses.0"},
		{"address CIDR", func(c *DNSConfig) { c.Nameservers[0].Addresses = []string{"192.0.2.1/32"} }, "nameservers.0.addresses.0"},
		{"scoped IPv6", func(c *DNSConfig) { c.Nameservers[0].Addresses = []string{"fe80::1%eth0"} }, "nameservers.0.addresses.0"},
		{"duplicate host", func(c *DNSConfig) {
			c.Nameservers = append(c.Nameservers, DNSNameserver{Host: strings.ToUpper(testNSName) + ".", Addresses: []string{"192.0.2.2"}})
		}, "nameservers.1.host"},
		{"duplicate address", func(c *DNSConfig) { c.Nameservers[0].Addresses = []string{"192.0.2.1", "::ffff:192.0.2.1"} }, "nameservers.0.addresses.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dnsConfig := dnsTestConfig()
			tt.change(&dnsConfig)
			_, err := ValidateDNSConfig(dnsConfig, testDomain)
			var invalid *DNSConfigError
			require.ErrorAs(t, err, &invalid)
			require.Equal(t, tt.field, invalid.Field)
		})
	}
}

func TestDecodeDNSConfigRequiresUnderstoodDocument(t *testing.T) {
	record := dnsTestRecord(t, 1, dnsTestConfig())
	_, err := DecodeDNSConfig(record, testDomain)
	require.NoError(t, err)

	for _, tt := range []struct {
		name    string
		record  config.Record
		message string
	}{
		{"other document", mustDNSRawRecord(t, "other", 1, record.Value), "expected shared DNS"},
		{"future schema", mustDNSRawRecord(t, DNSConfigName, 2, record.Value), "unsupported DNS configuration schema"},
		{"future field", mustDNSRawRecord(t, DNSConfigName, 1, []byte(`{"zone":"acme.example.com","nameservers":[{"host":"ns.acme.example.com","addresses":["192.0.2.1"]}],"future":true}`)), "unknown field"},
		{"future nested field", mustDNSRawRecord(t, DNSConfigName, 1, []byte(`{"zone":"acme.example.com","nameservers":[{"host":"ns.acme.example.com","addresses":["192.0.2.1"],"future":true}]}`)), "unknown field"},
		{"trailing object", mustDNSRawRecord(t, DNSConfigName, 1, append(append([]byte(nil), record.Value...), []byte(` {}`)...)), "one JSON value"},
		{"invalid JSON", mustDNSRawRecord(t, DNSConfigName, 1, []byte(`{"zone":`)), "invalid DNS configuration"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeDNSConfig(tt.record, testDomain)
			require.ErrorContains(t, err, tt.message)
		})
	}
	record.Value[0] = '['
	_, err = DecodeDNSConfig(record, testDomain)
	require.ErrorContains(t, err, "digest mismatch")
}

func mustDNSRawRecord(t *testing.T, name string, schema uint64, value []byte) config.Record {
	t.Helper()
	record, err := config.NewRecord(name, schema, 1, value)
	require.NoError(t, err)
	return record
}
