package overlay

import (
	"context"
	"fmt"
	"time"

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

func (t *QUIC) reuseConnection(ctx context.Context, q quic.EarlyConnection, s quic.Stream, dir direction) (*nodeConnection, bool, error) {
	negotiation := &protocol.Connection{
		Identity: t.Endpoint,
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
	}

	negotiation.Reset()

	rUnlock := t.cachedMutex.RLock(qKey)
	cache, cached := t.cachedConnections.Load(qKey)
	if cached {
		negotiation.CacheState = protocol.Connection_CACHED
		if cache.direction == directionIncoming {
			negotiation.CacheDirection = protocol.Connection_INCOMING
		} else {
			negotiation.CacheDirection = protocol.Connection_OUTGOING
		}
	} else {
		negotiation.CacheState = protocol.Connection_FRESH
		if dir == directionIncoming {
			negotiation.CacheDirection = protocol.Connection_INCOMING
		} else {
			negotiation.CacheDirection = protocol.Connection_OUTGOING
		}
	}
	rUnlock()

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

	// I really should make a state machine for this
	switch negotiation.CacheState {
	case protocol.Connection_CACHED:
		switch negotiation.CacheDirection {
		case protocol.Connection_INCOMING:
			if cached {
				if cache.direction == directionIncoming {
					// other: cached incoming
					//    us: cached incoming
					return nil, false, wrapReuseError("both peers have cached incoming connections")
				} else {
					// other: cached incoming
					//    us: cached outgoing
					fresh.quic.CloseWithError(508, wrapReuseError("previously cached connection was reused").Error())
					return cache, true, nil
				}
			} else {
				if dir == directionIncoming {
					// other: cached incoming
					//    us:    new incoming
					return nil, false, wrapReuseError("both peers have incoming connections")
				} else {
					// other: cached incoming
					//    us:    new outgoing
					return nil, false, wrapReuseError("other peer has cached connection while we are establishing a new outgoing connection")
				}
			}
		case protocol.Connection_OUTGOING:
			if cached {
				if cache.direction == directionIncoming {
					// other: cached outgoing
					//    us: cached incoming
					// we will let the receiver side close the connection
					return cache, true, nil
				} else {
					// other: cached outgoing
					//    us: cached outgoing
					return nil, false, wrapReuseError("both peers have cached outgoing connections")
				}
			} else {
				if dir == directionIncoming {
					// other: cached outgoing
					//    us:    new incoming
					return nil, false, wrapReuseError("other peer has cached connection while we are handling a new incoming connection")
				} else {
					// other: cached outgoing
					//    us:    new outgoing
					return nil, false, wrapReuseError("both peers have outgoing connections")
				}
			}
		default:
			return nil, false, fmt.Errorf("unknown transport cache direction")
		}
	case protocol.Connection_FRESH:
		switch negotiation.CacheDirection {
		case protocol.Connection_INCOMING:
			if cached {
				if cache.direction == directionIncoming {
					// other:    new incoming
					//    us: cached incoming
					return nil, false, wrapReuseError("both peers have cached incoming connections")
				} else {
					// other:    new incoming
					//    us: cached outgoing
					return nil, false, wrapReuseError("we have cached connection while peer is handling a new incoming connection")
				}
			} else {
				if dir == directionIncoming {
					// other:    new incoming
					//    us:    new incoming
					return nil, false, wrapReuseError("both peers have incoming connections")
				} else {
					// need to check again because we released the lock
					cache, cached = t.cachedConnections.Load(qKey)
					if cached {
						// other:    new incoming
						//    us: cached
						fresh.quic.CloseWithError(508, wrapReuseError("previously cached connection was reused").Error())
						return cache, true, nil
					} else {
						// other:    new incoming
						//    us:    new outgoing
						t.cachedConnections.Store(qKey, fresh)
						return fresh, false, nil
					}
				}
			}
		case protocol.Connection_OUTGOING:
			if cached {
				if cache.direction == directionIncoming {
					// other:    new outgoing
					//    us: cached incoming
					return nil, false, wrapReuseError("we have cached connection while peer is establishing a new outgoing connection")
				} else {
					// other:    new outgoing
					//    us: cached outgoing
					return nil, false, wrapReuseError("both peers have outgoing connections")
				}
			} else {
				if dir == directionIncoming {
					// need to check again because we released the lock
					cache, cached = t.cachedConnections.Load(qKey)
					if cached {
						// other:    new outgoing
						//    us: cached
						fresh.quic.CloseWithError(508, wrapReuseError("previously cached connection was reused").Error())
						return cache, true, nil
					} else {
						// other:    new outgoing
						//    us:    new incoming
						t.cachedConnections.Store(qKey, fresh)
						return fresh, false, nil
					}
				} else {
					// other:    new outgoing
					//    us:    new outgoing
					return nil, false, wrapReuseError("both peers have outgoing connections")
				}
			}
		default:
			return nil, false, fmt.Errorf("unknown transport cache direction")
		}
	default:
		return nil, false, fmt.Errorf("unknown transport cache state")
	}
}
