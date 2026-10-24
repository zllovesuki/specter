// Package waf provides request protection for public HTTP tunnels.
package waf

import (
	"bytes"
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"

	"google.golang.org/protobuf/proto"
)

const (
	ConfigName     = "waf"
	ConfigSchema   = 1
	MaxExclusions  = 32
	MaxPolicyBytes = 48 << 10
)

var releaseVersion = regexp.MustCompile(`^4\.[0-9]+\.[0-9]+$`)

func DefaultSettings() *protocol.WAFSettings {
	return &protocol.WAFSettings{DefaultMode: protocol.WAFPolicy_BLOCK, Channel: protocol.WAFSettings_LTS}
}

// ValidateSettings returns an owned copy suitable for immutable publication.
func ValidateSettings(s *protocol.WAFSettings) (*protocol.WAFSettings, error) {
	if s == nil {
		return nil, fmt.Errorf("settings are required")
	}
	if s.DefaultMode != protocol.WAFPolicy_BLOCK && s.DefaultMode != protocol.WAFPolicy_MONITOR {
		return nil, fmt.Errorf("default_mode must be BLOCK or MONITOR")
	}
	if s.Channel != protocol.WAFSettings_LTS && s.Channel != protocol.WAFSettings_STABLE {
		return nil, fmt.Errorf("channel must be LTS or STABLE")
	}
	if s.PinnedVersion != "" && !validReleaseVersion(s.PinnedVersion) {
		return nil, fmt.Errorf("pinned_version must be a CRS 4 release, such as 4.25.2")
	}
	return proto.Clone(s).(*protocol.WAFSettings), nil
}

func DecodeSettings(record config.Record) (*protocol.WAFSettings, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	if record.Name != ConfigName || record.Version.Schema != ConfigSchema {
		return nil, fmt.Errorf("unsupported WAF configuration")
	}
	s := new(protocol.WAFSettings)
	if err := DecodeJSON(bytes.NewReader(record.Value), s); err != nil {
		return nil, err
	}
	return ValidateSettings(s)
}

func DefaultPolicy() *protocol.WAFPolicy {
	return &protocol.WAFPolicy{Mode: protocol.WAFPolicy_INHERIT}
}

// ValidatePolicy returns an owned, normalized copy. Callers retain their input.
func ValidatePolicy(p *protocol.WAFPolicy) (*protocol.WAFPolicy, error) {
	if p == nil {
		return nil, fmt.Errorf("policy is required")
	}
	switch p.Mode {
	case protocol.WAFPolicy_INHERIT, protocol.WAFPolicy_BLOCK, protocol.WAFPolicy_MONITOR, protocol.WAFPolicy_OFF:
	default:
		return nil, fmt.Errorf("mode must be INHERIT, BLOCK, MONITOR, or OFF")
	}
	if len(p.Exclusions) > MaxExclusions {
		return nil, fmt.Errorf("at most %d rule exceptions are allowed", MaxExclusions)
	}
	for _, e := range p.Exclusions {
		if e == nil || e.RuleId < 100000 || e.RuleId > 999999 {
			return nil, fmt.Errorf("rule_id must be a six-digit rule ID")
		}
		if len(e.Path) > 1024 || (e.Path != "" && !strings.HasPrefix(e.Path, "/")) || strings.ContainsAny(e.Path, "?#") || strings.ContainsFunc(e.Path, unicode.IsControl) {
			return nil, fmt.Errorf("path must be an exact URL path without a query or fragment, up to 1024 bytes")
		}
	}
	p = proto.Clone(p).(*protocol.WAFPolicy)
	slices.SortFunc(p.Exclusions, func(a, b *protocol.WAFExclusion) int {
		if a.RuleId != b.RuleId {
			return cmp.Compare(a.RuleId, b.RuleId)
		}
		return strings.Compare(a.Path, b.Path)
	})
	p.Exclusions = slices.CompactFunc(p.Exclusions, func(a, b *protocol.WAFExclusion) bool {
		return a.RuleId == b.RuleId && a.Path == b.Path
	})
	encoded, err := MarshalJSON(p)
	if err != nil || len(encoded) > MaxPolicyBytes {
		return nil, fmt.Errorf("rule exceptions exceed the %d-byte policy limit", MaxPolicyBytes)
	}
	return p, nil
}

func EffectiveMode(p *protocol.WAFPolicy, defaultMode protocol.WAFPolicy_Mode) protocol.WAFPolicy_Mode {
	if p.GetMode() == protocol.WAFPolicy_INHERIT {
		return defaultMode
	}
	return p.GetMode()
}

func DisabledRules(p *protocol.WAFPolicy, path string) []int {
	var ids []int
	for _, e := range p.GetExclusions() {
		if e.Path == "" || e.Path == path {
			ids = append(ids, int(e.RuleId))
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
