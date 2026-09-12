package server

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"time"

	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/twitchtv/twirp"
)

func sessionAttachment(ctx context.Context) (transport.PhysicalConn, *x509.Certificate, error) {
	d := rpc.GetDelegation(ctx)
	if d == nil || d.Certificate == nil {
		return nil, nil, twirp.Unauthenticated.Error("missing client certificate")
	}
	if _, err := pki.ExtractCertificateIdentity(d.Certificate); err != nil {
		return nil, nil, twirp.Unauthenticated.Error(err.Error())
	}
	provider, ok := d.Conn.(transport.PhysicalConnProvider)
	if !ok || provider.PhysicalConn() == nil {
		return nil, nil, twirp.Unimplemented.Error("transport does not expose a physical connection")
	}
	return provider.PhysicalConn(), d.Certificate, nil
}

func (s *Server) reserveSession(sess *session) error {
	if err := s.sessions.reserve(sess); err != nil {
		var rpcError twirp.Error
		if !errors.As(err, &rpcError) || rpcError.Code() != twirp.FailedPrecondition {
			sess.fail(err.Error())
		}
		return err
	}
	return nil
}

func (s *Server) OpenEphemeralSession(ctx context.Context, _ *protocol.OpenEphemeralSessionRequest) (*protocol.OpenSessionResponse, error) {
	conn, cert, err := sessionAttachment(ctx)
	if err != nil {
		return nil, err
	}
	label, inv, err := tun.EphemeralLabel(s.Chord.ID(), cert.RawSubjectPublicKeyInfo)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	sess := &session{
		alias:        tun.SessionAlias(inv),
		hostname:     label,
		conn:         conn,
		mode:         ephemeral,
		spki:         cert.RawSubjectPublicKeyInfo,
		certNotAfter: cert.NotAfter,
	}
	if err := s.reserveSession(sess); err != nil {
		return nil, err
	}
	if !sess.activate(ctx, time.Time{}) {
		sess.fail("ephemeral activation canceled")
		return nil, twirp.Aborted.Error("ephemeral activation canceled")
	}
	return &protocol.OpenSessionResponse{
		Hostname: label,
		Apex:     s.Apex,
		Node:     s.TunnelTransport.Identity(),
	}, nil
}

func (s *Server) OpenDelegatedSession(ctx context.Context, req *protocol.OpenDelegatedSessionRequest) (resp *protocol.OpenSessionResponse, err error) {
	conn, cert, err := sessionAttachment(ctx)
	if err != nil {
		return nil, err
	}
	slot := req.GetRouteSlot()
	if slot < 1 || slot > tun.NumRedundantLinks {
		return nil, twirp.InvalidArgument.Errorf("route_slot must be between 1 and %d", tun.NumRedundantLinks)
	}
	var inv [16]byte
	if _, err := rand.Read(inv[:]); err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	sess := &session{
		alias:        tun.SessionAlias(inv),
		conn:         conn,
		mode:         delegated,
		certNotAfter: cert.NotAfter,
	}
	if err := s.reserveSession(sess); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			sess.fail(err.Error())
		}
	}()
	secret, err := tun.ParseDelegationToken(req.GetToken())
	if err != nil {
		return nil, twirp.InvalidArgument.Error(err.Error())
	}
	start := time.Now()
	rec, err := s.readDelegation(ctx, tun.DelegationID(secret))
	if errors.Is(err, errDelegationNotFound) || errors.Is(err, errDelegationInvalid) {
		return nil, twirp.Unauthenticated.Error(err.Error())
	}
	if err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	if rec.GetExpiresAt() != 0 && rec.GetExpiresAt() <= start.Unix() {
		return nil, twirp.PermissionDenied.Error("grant has expired")
	}
	sess.mu.Lock()
	sess.hostname, sess.grantID, sess.owner = rec.GetHostname(), rec.GetId(), rec.GetOwner()
	if rec.GetExpiresAt() != 0 {
		sess.grantExpiry = time.Unix(rec.GetExpiresAt(), 0)
	}
	sess.mu.Unlock()
	leaseKey := []byte(tun.ClientLeaseKey(rec.GetOwner()))
	lease, err := s.Chord.Acquire(ctx, leaseKey, 30*time.Second)
	if err != nil {
		return nil, twirp.Unavailable.Errorf("acquiring owner lease: %v", err)
	}
	defer s.Chord.Release(ctx, leaseKey, lease)
	if err := s.verifyDelegatedAuthority(ctx, rec); err != nil {
		return nil, delegationAuthorityError(err)
	}
	route := &protocol.TunnelRoute{
		ClientDestination: &protocol.Node{
			Address:    sess.alias,
			Rendezvous: true,
		},
		ChordDestination:  s.ChordTransport.Identity(),
		TunnelDestination: s.TunnelTransport.Identity(),
		Hostname:          sess.hostname,
	}
	data, err := route.MarshalVT()
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := s.Chord.Put(publishCtx, []byte(tun.RoutingKey(sess.hostname, int(slot))), data); err != nil {
		return nil, twirp.Unavailable.Errorf("publishing slot %d failed: %v", slot, err)
	}
	if !sess.activate(ctx, sess.authorizationDeadline(start, sess.grantExpiry)) {
		return nil, twirp.Aborted.Error("activation canceled or authorization expired; the published route now names a dead session")
	}
	go s.maintainSession(s.ParentContext, sess)
	return &protocol.OpenSessionResponse{
		Hostname:  sess.hostname,
		Apex:      s.Apex,
		GrantId:   rec.GetId(),
		ExpiresAt: rec.GetExpiresAt(),
		Node:      s.TunnelTransport.Identity(),
	}, nil
}
