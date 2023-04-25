package acme

import (
	"context"
	"fmt"

	acmeSpec "kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/tun"

	"github.com/mholt/acmez"
	"github.com/mholt/acmez/acme"
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
	var token []byte
	if !s.isManaged(zone) {
		bundle, err := tun.FindCustomHostname(ctx, s.KV, zone)
		if err != nil {
			return "", err
		}
		token = bundle.GetClientToken().GetToken()
	}
	return acmeSpec.EncodeZone(zone, token), nil
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
