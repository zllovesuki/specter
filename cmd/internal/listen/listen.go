package listen

import (
	"fmt"
	"net"
	"strings"
)

type IPVersion int

const (
	IPAny IPVersion = iota
	IPV4
	IPV6
)

const (
	// FlyGlobalServicesHost is a special-cased hostname used by Fly.io's
	// anycast load balancer. UDP traffic is only delivered on IPv4, even if
	// AAAA records exist, so we force an IPv4 bind when this host is used.
	FlyGlobalServicesHost = "fly-global-services"
)

type Address struct {
	Address string
	Host    string
	Network string
	Version IPVersion
}

// ParseAddresses normalizes listen addresses and expands per-family networks.
func ParseAddresses(proto, base string, overrides []string) ([]Address, error) {
	addrs := []string{base}
	if len(overrides) > 0 {
		addrs = make([]string, 0, len(overrides))
		for _, o := range overrides {
			v := strings.TrimSpace(o)
			if v == "" {
				continue
			}
			addrs = append(addrs, v)
		}
		if len(addrs) == 0 {
			addrs = []string{base}
		}
	}

	seen := make(map[string]struct{}, len(addrs))
	out := make([]Address, 0, len(addrs))
	for _, a := range addrs {
		if _, ok := seen[a]; ok {
			continue
		}
		host, _, err := net.SplitHostPort(a)
		if err != nil {
			return nil, err
		}
		if net.ParseIP(host) == nil && host != FlyGlobalServicesHost {
			return nil, fmt.Errorf("listen host must be an IP address (got %q)", host)
		}
		seen[a] = struct{}{}
		version := overrideHostIPVersion(host, ClassifyIPVersion(host))
		out = append(out, Address{
			Address: a,
			Host:    host,
			Version: version,
			Network: NetworkForVersion(proto, version),
		})
	}

	return out, nil
}

func ClassifyIPVersion(host string) IPVersion {
	ip := net.ParseIP(host)
	if ip == nil {
		return IPAny
	}
	if ip.To4() != nil {
		return IPV4
	}
	return IPV6
}

// overrideHostIPVersion applies known host-specific IP family constraints.
func overrideHostIPVersion(host string, version IPVersion) IPVersion {
	if host == FlyGlobalServicesHost {
		// Fly UDP proxy is IPv4-only; force v4 even if the hostname resolves to v6.
		return IPV4
	}
	return version
}

func NetworkForVersion(proto string, version IPVersion) string {
	switch version {
	case IPV4:
		return proto + "4"
	case IPV6:
		return proto + "6"
	default:
		return proto
	}
}
