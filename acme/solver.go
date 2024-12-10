package acme

import (
	"context"
	"fmt"

	acmeSpec "go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/tun"

	"github.com/mholt/acmez/v2"
	"github.com/mholt/acmez/v2/acme"
)

type ChordSolver struct {
	KV             chord.KV
	ManagedDomains []string
}

var _ acmez.Solver = (*ChordSolver)(nil)

func (s *ChordSolver) isManaged(zone string) bool {
	for _, d := range s.ManagedDomains {
		if zone == d {
			return true
		}
	}
	return false
}

func (s *ChordSolver) getSubdomain(ctx context.Context, zone string) (string, error) {
	if s.isManaged(zone) {
		return acmeSpec.ManagedDelegation, nil
	}
	bundle, err := tun.FindCustomHostname(ctx, s.KV, zone)
	if err != nil {
		return "", err
	}
	token := bundle.GetClientToken().GetToken()
	return acmeSpec.EncodeClientToken(token), nil
}

func (s *ChordSolver) Present(ctx context.Context, chal acme.Challenge) error {
	if chal.Identifier.Type != "dns" {
		return fmt.Errorf("dns: unexpected challenge type: %s", chal.Identifier.Type)
	}
	subdomain, err := s.getSubdomain(ctx, chal.Identifier.Value)
	if err != nil {
		return err
	}
	return s.KV.PrefixAppend(ctx, []byte(dnsKeyName(subdomain)), []byte(chal.DNS01KeyAuthorization()))
}

func (s *ChordSolver) CleanUp(ctx context.Context, chal acme.Challenge) error {
	if chal.Identifier.Type != "dns" {
		return fmt.Errorf("dns: unexpected challenge type: %s", chal.Identifier.Type)
	}
	subdomain, err := s.getSubdomain(ctx, chal.Identifier.Value)
	if err != nil {
		return err
	}
	return s.KV.PrefixRemove(ctx, []byte(dnsKeyName(subdomain)), []byte(chal.DNS01KeyAuthorization()))
}
