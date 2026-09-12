package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/promise"

	"github.com/twitchtv/twirp"
)

var (
	errDelegationNotFound = errors.New("unknown or revoked grant")
	errDelegationInvalid  = errors.New("invalid grant record")
	errDelegationNotOwned = errors.New("hostname is not registered to the grant owner")
)

func (s *Server) readDelegation(ctx context.Context, id string) (*protocol.DelegationRecord, error) {
	if !tun.IsDelegationID(id) {
		return nil, errDelegationInvalid
	}
	data, err := s.Chord.Get(ctx, []byte(tun.DelegationKey(id)))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errDelegationNotFound
	}
	rec := &protocol.DelegationRecord{}
	if rec.UnmarshalVT(data) != nil || rec.GetVersion() != tun.DelegationRecordVersion || rec.GetId() != id || len(rec.GetOwner().GetToken()) == 0 || rec.GetHostname() == "" || rec.GetExpiresAt() < 0 {
		return nil, errDelegationInvalid
	}
	return rec, nil
}

func (s *Server) verifyDelegatedAuthority(ctx context.Context, rec *protocol.DelegationRecord) error {
	owned, err := s.Chord.PrefixContains(ctx, []byte(tun.ClientHostnamesPrefix(rec.GetOwner())), []byte(rec.GetHostname()))
	if err != nil {
		return err
	}
	if !owned {
		return errDelegationNotOwned
	}
	if strings.Contains(rec.GetHostname(), ".") {
		custom, err := tun.FindCustomHostname(ctx, s.Chord, rec.GetHostname())
		if errors.Is(err, tun.ErrHostnameNotFound) {
			return errDelegationNotOwned
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(custom.GetClientToken().GetToken(), rec.GetOwner().GetToken()) {
			return errDelegationNotOwned
		}
	}
	return nil
}

func delegationAuthorityError(err error) error {
	if errors.Is(err, errDelegationNotOwned) {
		return twirp.PermissionDenied.Error(err.Error())
	}
	return twirp.Unavailable.Error(err.Error())
}

func normalizeDelegationHostname(hostname string) (string, error) {
	if strings.Contains(hostname, ".") {
		return acme.Normalize(hostname)
	}
	// Short labels must match a server-generated owner registration exactly.
	return hostname, nil
}

func grantFromRecord(rec *protocol.DelegationRecord) *protocol.DelegationGrant {
	return &protocol.DelegationGrant{
		Id:        rec.GetId(),
		Hostname:  rec.GetHostname(),
		ExpiresAt: rec.GetExpiresAt(),
	}
}

func (s *Server) MintDelegation(ctx context.Context, req *protocol.MintDelegationRequest) (*protocol.MintDelegationResponse, error) {
	owner, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	hostname, err := normalizeDelegationHostname(req.GetHostname())
	if err != nil {
		return nil, twirp.InvalidArgument.Error(err.Error())
	}
	if req.GetExpiresAt() != 0 && req.GetExpiresAt() <= time.Now().Unix() {
		return nil, twirp.InvalidArgument.Error("expires_at must be in the future")
	}
	rec := &protocol.DelegationRecord{
		Version:   tun.DelegationRecordVersion,
		Owner:     owner,
		Hostname:  hostname,
		ExpiresAt: req.GetExpiresAt(),
	}
	if err := s.verifyDelegatedAuthority(ctx, rec); err != nil {
		return nil, delegationAuthorityError(err)
	}
	index := []byte(tun.ClientDelegationsPrefix(owner))
	ids, err := s.Chord.PrefixList(ctx, index)
	if err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	if len(ids) >= tun.MaxDelegationsPerOwner {
		return nil, twirp.ResourceExhausted.Error("delegation limit reached; revoke unused or incomplete grants")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	rec.Id = tun.DelegationID(secret)
	data, err := rec.MarshalVT()
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	if err := s.Chord.PrefixAppend(ctx, index, []byte(rec.Id)); err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	if err := s.Chord.Put(ctx, []byte(tun.DelegationKey(rec.Id)), data); err != nil {
		return nil, twirp.Unavailable.Errorf("grant %s may be incomplete: %v", rec.Id, err)
	}
	return &protocol.MintDelegationResponse{
		Grant: grantFromRecord(rec),
		Token: tun.EncodeDelegationToken(secret),
	}, nil
}

func (s *Server) ListDelegations(ctx context.Context, _ *protocol.ListDelegationsRequest) (*protocol.ListDelegationsResponse, error) {
	owner, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := s.Chord.PrefixList(ctx, []byte(tun.ClientDelegationsPrefix(owner)))
	if err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	if len(ids) > tun.MaxDelegationsPerOwner {
		ids = ids[:tun.MaxDelegationsPerOwner]
	}
	resp := &protocol.ListDelegationsResponse{}
	for offset := 0; offset < len(ids); offset += 8 {
		batch := ids[offset:min(offset+8, len(ids))]
		jobs := make([]func(context.Context) (*protocol.DelegationGrant, error), len(batch))
		for i, raw := range batch {
			id := string(raw)
			jobs[i] = func(ctx context.Context) (*protocol.DelegationGrant, error) {
				rec, err := s.readDelegation(ctx, id)
				if errors.Is(err, errDelegationNotFound) || errors.Is(err, errDelegationInvalid) || (err == nil && !bytes.Equal(rec.GetOwner().GetToken(), owner.GetToken())) {
					return &protocol.DelegationGrant{
						Id:         id,
						Incomplete: true,
					}, nil
				}
				if err != nil {
					return nil, err
				}
				return grantFromRecord(rec), nil
			}
		}
		grants, errs := promise.All(ctx, jobs...)
		for _, err := range errs {
			if err != nil {
				return nil, twirp.Unavailable.Error(err.Error())
			}
		}
		resp.Grants = append(resp.Grants, grants...)
	}
	return resp, nil
}

func (s *Server) RevokeDelegation(ctx context.Context, req *protocol.RevokeDelegationRequest) (*protocol.RevokeDelegationResponse, error) {
	owner, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	id := req.GetId()
	if !tun.IsDelegationID(id) {
		return nil, twirp.InvalidArgument.Error("invalid grant ID")
	}
	rec, err := s.readDelegation(ctx, id)
	if err != nil && !errors.Is(err, errDelegationNotFound) && !errors.Is(err, errDelegationInvalid) {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	index := []byte(tun.ClientDelegationsPrefix(owner))
	if rec == nil || !bytes.Equal(rec.GetOwner().GetToken(), owner.GetToken()) {
		indexed, err := s.Chord.PrefixContains(ctx, index, []byte(id))
		if err != nil {
			return nil, twirp.Unavailable.Error(err.Error())
		}
		if !indexed {
			return nil, twirp.NotFound.Error("grant is not owned by this client")
		}
	}
	if err := s.Chord.Delete(ctx, []byte(tun.DelegationKey(id))); err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	resp := &protocol.RevokeDelegationResponse{Revoked: true}
	if err := s.Chord.PrefixRemove(ctx, index, []byte(id)); err != nil {
		resp.IndexError = err.Error()
	}
	return resp, nil
}
