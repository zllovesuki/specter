//go:build android
// +build android

package errata

import (
	"context"
	"net"
)

const bootstrapDNS = "1.1.1.1:53"

func ConfigDNS() bool {
	var dialer net.Dialer
	net.DefaultResolver = &net.Resolver{
		PreferGo: false,
		Dial: func(context context.Context, _, _ string) (net.Conn, error) {
			conn, err := dialer.DialContext(context, "udp", bootstrapDNS)
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
	}
	return true
}
