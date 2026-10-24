package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/waf"

	"github.com/twitchtv/twirp"
)

const maxHostWAFBytes = 64 << 10

func limitClientRPCBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(1 << 10)
		switch r.URL.Path {
		case protocol.TunnelServicePathPrefix + "GetHostWAF", protocol.TunnelServicePathPrefix + "SetHostWAF":
			limit = maxHostWAFBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func normalizeWAFHostname(hostname string) (string, error) {
	if hostname == "" || len(hostname) > 253 {
		return "", twirp.InvalidArgument.Error("hostname is required and must fit a DNS name")
	}
	hostname, err := normalizeDelegationHostname(hostname)
	if err != nil {
		return "", twirp.InvalidArgument.Error(err.Error())
	}
	return hostname, nil
}

func hostWAFVersion(hostname string, data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(append([]byte(hostname+"\x00"), data...))
	return hex.EncodeToString(sum[:])
}

func (s *Server) readHostWAF(ctx context.Context, hostname string) (*protocol.HostWAFRecord, string, error) {
	data, err := s.Chord.Get(ctx, []byte(tun.HostWAFKey(hostname)))
	if err != nil || len(data) == 0 {
		return nil, "", err
	}
	if len(data) > maxHostWAFBytes {
		return nil, "", fmt.Errorf("saved WAF policy exceeds size limit")
	}
	var record protocol.HostWAFRecord
	if err := waf.DecodeJSON(bytes.NewReader(data), &record); err != nil {
		return nil, "", fmt.Errorf("invalid saved WAF policy: %w", err)
	}
	if record.Schema != 1 || len(record.Owner) == 0 {
		return nil, "", fmt.Errorf("unsupported saved WAF policy")
	}
	policy, err := waf.ValidatePolicy(record.Policy)
	if err != nil {
		return nil, "", fmt.Errorf("invalid saved WAF policy: %w", err)
	}
	record.Policy = policy
	return &record, hostWAFVersion(hostname, data), nil
}

func (s *Server) requireWAFHostname(ctx context.Context, owner *protocol.ClientToken, hostname string) error {
	owned, err := s.ownsHostname(ctx, owner, hostname)
	if err != nil {
		return twirp.Unavailable.Errorf("could not verify hostname ownership: %v", err)
	}
	if !owned {
		return twirp.PermissionDenied.Error("hostname is not registered to this client")
	}
	return nil
}

func hostWAFResponse(hostname, version string, record *protocol.HostWAFRecord) *protocol.HostWAFResponse {
	policy := waf.DefaultPolicy()
	if record != nil {
		policy = record.Policy
	}
	return &protocol.HostWAFResponse{Hostname: hostname, Version: version, Policy: policy}
}

func (s *Server) GetHostWAF(ctx context.Context, req *protocol.GetHostWAFRequest) (*protocol.HostWAFResponse, error) {
	owner, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	hostname, err := normalizeWAFHostname(req.GetHostname())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	if err := s.requireWAFHostname(ctx, owner, hostname); err != nil {
		return nil, err
	}
	record, version, err := s.readHostWAF(ctx, hostname)
	if err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	if record != nil && !bytes.Equal(record.Owner, owner.GetToken()) {
		record, version = nil, ""
	}
	return hostWAFResponse(hostname, version, record), nil
}

func (s *Server) SetHostWAF(ctx context.Context, req *protocol.SetHostWAFRequest) (*protocol.HostWAFResponse, error) {
	owner, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	hostname, err := normalizeWAFHostname(req.GetHostname())
	if err != nil {
		return nil, err
	}
	input := req.GetPolicy()
	if input == nil {
		return nil, twirp.InvalidArgument.Error("policy is required")
	}
	policy, err := waf.ValidatePolicy(input)
	if err != nil {
		return nil, twirp.InvalidArgument.Error(err.Error())
	}
	record := &protocol.HostWAFRecord{Schema: 1, Owner: owner.GetToken(), Policy: policy}
	data, err := waf.MarshalJSON(record)
	if err != nil || len(data) > maxHostWAFBytes {
		return nil, twirp.InvalidArgument.Error("WAF policy exceeds size limit")
	}

	// Policy writes and hostname release share the existing owner's lease. Keep
	// validation outside it, and bound the read/check/write well below its TTL.
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	leaseKey := []byte(tun.ClientLeaseKey(owner))
	lease, err := s.Chord.Acquire(ctx, leaseKey, 30*time.Second)
	if err != nil {
		return nil, twirp.Unavailable.Errorf("could not acquire owner lease: %v", err)
	}
	defer s.Chord.Release(ctx, leaseKey, lease)
	if err := s.requireWAFHostname(ctx, owner, hostname); err != nil {
		return nil, err
	}
	previous, version, err := s.readHostWAF(ctx, hostname)
	if err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	if previous != nil && !bytes.Equal(previous.Owner, owner.GetToken()) {
		version = ""
	}
	if version != req.GetExpectedVersion() {
		return nil, twirp.FailedPrecondition.Error("WAF settings changed; refresh before saving again")
	}
	if err := s.Chord.Put(ctx, []byte(tun.HostWAFKey(hostname)), data); err != nil {
		return nil, twirp.Unavailable.Errorf("WAF save result is uncertain; refresh to review saved settings: %v", err)
	}
	return hostWAFResponse(hostname, hostWAFVersion(hostname, data), record), nil
}

// WAFPolicy resolves request policy without retaining stale ownership authority.
// The usual inherited profile needs no owner lookups; an override does.
func (s *Server) WAFPolicy(ctx context.Context, hostname string) (*protocol.WAFPolicy, error) {
	inherit := waf.DefaultPolicy()
	if _, _, ephemeral := tun.ParseEphemeralLabel(hostname); ephemeral {
		return inherit, nil
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	record, _, err := s.readHostWAF(ctx, hostname)
	if err != nil || record == nil {
		return inherit, err
	}
	if record.Policy.Mode == protocol.WAFPolicy_INHERIT && len(record.Policy.Exclusions) == 0 {
		return inherit, nil
	}
	owned, err := s.ownsHostname(ctx, &protocol.ClientToken{Token: record.Owner}, hostname)
	if err != nil || !owned {
		return inherit, err
	}
	return record.Policy, nil
}

func (s *Server) clearHostWAF(ctx context.Context, owner *protocol.ClientToken, hostname string) error {
	record, _, err := s.readHostWAF(ctx, hostname)
	if err != nil || record == nil || !bytes.Equal(record.Owner, owner.GetToken()) {
		return err
	}
	return s.Chord.Delete(ctx, []byte(tun.HostWAFKey(hostname)))
}
