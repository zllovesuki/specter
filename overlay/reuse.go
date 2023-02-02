package overlay

import (
	"context"
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

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
	err = rpc.BoundedReceive(s, negotiation, 1024)
	if err != nil {
		return nil, false, fmt.Errorf("error receiving identity: %w", err)
	}
	s.SetReadDeadline(time.Time{})

	qKey := makeCachedKey(negotiation.GetIdentity())
	fresh := &nodeConnection{
		peer:      negotiation.GetIdentity(),
		quic:      q,
		direction: dir,
	}

	if t.Endpoint.GetId() == negotiation.GetIdentity().GetId() {
		t.Logger.Debug("connecting to self, skipping connection reuse", zap.String("direction", dir.String()))
		return fresh, false, nil
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
					return nil, false, fmt.Errorf("invalid state: both peers have cached incoming connections")
				} else {
					// other: cached incoming
					//    us: cached outgoing
					fresh.quic.CloseWithError(508, "Previously cached connection was reused")
					return cache, true, nil
				}
			} else {
				if dir == directionIncoming {
					// other: cached incoming
					//    us:    new incoming
					return nil, false, fmt.Errorf("invalid state: both peers have incoming connections")
				} else {
					// other: cached incoming
					//    us:    new outgoing
					return nil, false, fmt.Errorf("invalid state: other peer has cached connection while we are establishing a new outgoing connection")
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
					return nil, false, fmt.Errorf("invalid state: both peers have cached outgoing connections")
				}
			} else {
				if dir == directionIncoming {
					// other: cached outgoing
					//    us:    new incoming
					return nil, false, fmt.Errorf("invalid state: other peer has cached connection while we are handling a new incoming connection")
				} else {
					// other: cached outgoing
					//    us:    new outgoing
					return nil, false, fmt.Errorf("invalid state: both peers have outgoing connections")
				}
			}
		default:
			return nil, false, fmt.Errorf("invalid state: unknown transport cache direction")
		}
	case protocol.Connection_FRESH:
		switch negotiation.CacheDirection {
		case protocol.Connection_INCOMING:
			if cached {
				if cache.direction == directionIncoming {
					// other:    new incoming
					//    us: cached incoming
					return nil, false, fmt.Errorf("invalid state: both peers have cached incoming connections")
				} else {
					// other:    new incoming
					//    us: cached outgoing
					return nil, false, fmt.Errorf("invalid state: we have cached connection while peer is handling a new incoming connection")
				}
			} else {
				if dir == directionIncoming {
					// other:    new incoming
					//    us:    new incoming
					return nil, false, fmt.Errorf("invalid state: both peers have incoming connections")
				} else {
					// other:    new incoming
					//    us:    new outgoing
					// need to check again because we released the lock
					cache, cached = t.cachedConnections.Load(qKey)
					if cached {
						fresh.quic.CloseWithError(508, "Previously cached connection was reused")
						return cache, true, nil
					} else {
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
					// it is likely that concurrent reuse was issued by the other peer, therefore they should
					// try again with the cached connection
					return nil, false, fmt.Errorf("invalid state: we have cached connection while peer is establishing a new outgoing connection")
				} else {
					// other:    new outgoing
					//    us: cached outgoing
					return nil, false, fmt.Errorf("invalid state: both peers have outgoing connections")
				}
			} else {
				if dir == directionIncoming {
					// other:    new outgoing
					//    us:    new incoming
					// need to check again because we released the lock
					cache, cached = t.cachedConnections.Load(qKey)
					if cached {
						fresh.quic.CloseWithError(508, "Previously cached connection was reused")
						return cache, true, nil
					} else {
						t.cachedConnections.Store(qKey, fresh)
						return fresh, false, nil
					}
				} else {
					// other:    new outgoing
					//    us:    new outgoing
					return nil, false, fmt.Errorf("invalid state: both peers have outgoing connections")
				}
			}
		default:
			return nil, false, fmt.Errorf("invalid state: unknown transport cache direction")
		}
	default:
		return nil, false, fmt.Errorf("invalid state: unknown transport cache state")
	}
}
