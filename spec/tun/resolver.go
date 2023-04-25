package tun

import "context"

type DNSResolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
}
