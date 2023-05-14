package errata

import (
	"net"
	"time"

	"github.com/ncruces/go-dns"
)

func ConfigDNS(enabled bool) bool {
	if !enabled {
		return false
	}
	resolver, err := dns.NewDoHResolver(
		"https://cloudflare-dns.com/dns-query",
		dns.DoHAddresses("1.1.1.1", "2606:4700:4700::1111", "1.0.0.1", "2606:4700:4700::1001"),
		dns.DoHCache(dns.MaxCacheTTL(time.Second*30)),
	)
	if err != nil {
		return false
	}
	net.DefaultResolver = resolver
	return true
}
