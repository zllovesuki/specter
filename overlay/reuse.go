package overlay

import (
	"context"
	"fmt"
	"time"

	"go.miragespace.co/specter/spec"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

const (
	reuseErrorState = "invalid state"
)

func wrapReuseError(msg string) error {
	return fmt.Errorf("%s: %s", reuseErrorState, msg)
}

func (t *QUIC) acquireNegotiation(ctx context.Context, q *quic.Conn, key string) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := context.Cause(q.Context()); err != nil {
			return nil, err
		}
		pending, loaded := t.negotiations.LoadOrStoreLazy(key, func() chan struct{} { return make(chan struct{}) })
		if !loaded {
			return func() {
				// Only the holder removes this entry.
				t.negotiations.Delete(key)
				close(pending)
			}, nil
		}
		select {
		case <-pending:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-q.Context().Done():
			return nil, context.Cause(q.Context())
		}
	}
}

func (t *QUIC) reuseConnection(ctx context.Context, q *quic.Conn, s *quic.Stream, dir direction) (*nodeConnection, bool, error) {
	// Setup owns the fresh connection until cancellation is disarmed under
	// the cache lock. Once published, other callers may already be using it.
	stopSetup := context.AfterFunc(ctx, func() { q.CloseWithError(406, "Connection setup canceled") })
	defer stopSetup()

	negotiation := &protocol.Connection{
		Identity: t.Endpoint,
		Version:  spec.BuildVersion,
	}

	err := rpc.Send(s, negotiation)
	if err != nil {
		return nil, false, fmt.Errorf("error sending identity: %w", err)
	}
	negotiation.Reset()

	s.SetReadDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.BoundedReceive(s, negotiation, 256)
	if err != nil {
		return nil, false, fmt.Errorf("error receiving identity: %w", err)
	}
	s.SetReadDeadline(time.Time{})

	if t.UseCertificateIdentity {
		chain := q.ConnectionState().TLS.VerifiedChains
		if len(chain) == 0 || len(chain[0]) == 0 {
			return nil, false, transport.ErrNoCertificate
		}
		cert := chain[0][0]
		identity, err := pki.ExtractCertificateIdentity(cert)
		if err != nil {
			return nil, false, fmt.Errorf("extracting certificate identity: %w", err)
		}
		t.Logger.Debug("Using identify from certificate", zap.String("remote", q.RemoteAddr().String()), zap.Object("identity", identity))
		negotiation.Identity = identity.NodeIdentity()
	}

	qKey := t.makeCachedKey(negotiation.GetIdentity())
	fresh := &nodeConnection{
		peer:      negotiation.GetIdentity(),
		quic:      q,
		direction: dir,
		version:   negotiation.GetVersion(),
	}

	// Only one endpoint serializes negotiation, so competing connections cannot
	// each observe FRESH and publish different winners. The first winner stays
	// useful regardless of direction. Client identities come from certificates,
	// so their server owns arbitration rather than comparing token addresses.
	if t.UseCertificateIdentity || (t.VirtualTransport && t.Endpoint.GetAddress() > fresh.peer.GetAddress()) {
		release, err := t.acquireNegotiation(ctx, q, qKey)
		if err != nil {
			return nil, false, err
		}
		defer release()
	}

	negotiation.Reset()

	rUnlock := t.cachedMutex.RLock(qKey)
	cache, cached := t.cachedConnections.Load(qKey)
	selected := fresh
	negotiation.CacheState = protocol.Connection_FRESH
	if cached {
		negotiation.CacheState = protocol.Connection_CACHED
		selected = cache
	}
	negotiation.CacheDirection = protocol.Connection_INCOMING
	if selected.direction == directionOutgoing {
		negotiation.CacheDirection = protocol.Connection_OUTGOING
	}
	rUnlock()

	localState, localDirection := negotiation.CacheState, negotiation.CacheDirection
	err = rpc.Send(s, negotiation)
	if err != nil {
		return nil, false, fmt.Errorf("error sending cache status: %w", err)
	}
	negotiation.Reset()

	s.SetReadDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.BoundedReceive(s, negotiation, 8)
	if err != nil {
		return nil, false, fmt.Errorf("error receiving cache status: %w", err)
	}
	s.SetReadDeadline(time.Time{})

	unlock := t.cachedMutex.Lock(qKey)
	defer unlock()

	current, exists := t.cachedConnections.Load(qKey)
	if exists != cached || current != cache {
		return nil, false, wrapReuseError("cached connection changed during negotiation")
	}
	if cached && cache.quic.Context().Err() != nil {
		return nil, false, wrapReuseError("cached connection closed during negotiation")
	}

	if negotiation.CacheState != protocol.Connection_CACHED && negotiation.CacheState != protocol.Connection_FRESH {
		return nil, false, fmt.Errorf("unknown transport cache state")
	}
	if negotiation.CacheDirection != protocol.Connection_INCOMING && negotiation.CacheDirection != protocol.Connection_OUTGOING {
		return nil, false, fmt.Errorf("unknown transport cache direction")
	}
	// Agreement requires the same cache state and opposite ends of one connection.
	if negotiation.CacheState != localState || negotiation.CacheDirection == localDirection {
		return nil, false, wrapReuseError(fmt.Sprintf("peer reports %s/%s; local state is %s/%s",
			negotiation.CacheState, negotiation.CacheDirection, localState, localDirection))
	}
	if cached {
		// Only the outgoing side closes the unused candidate.
		if cache.direction == directionOutgoing {
			fresh.quic.CloseWithError(508, wrapReuseError("previously cached connection was reused").Error())
		}
		return cache, true, nil
	}
	if !stopSetup() {
		return nil, false, ctx.Err()
	}
	t.cachedConnections.Store(qKey, fresh)
	return fresh, false, nil
}
