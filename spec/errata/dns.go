package errata

import (
	"net"
	"time"

	"github.com/ncruces/go-dns"
)

func ConfigDNS() bool {
	resolver, err := dns.NewDoHResolver(
		"https://cloudflare-dns.com/dns-query",
		dns.DoHAddresses("1.1.1.1"),
		dns.DoHCache(dns.MaxCacheTTL(time.Second*30)),
	)
	if err != nil {
		return false
	}
	net.DefaultResolver = resolver
	return true
}
