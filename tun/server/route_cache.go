package server

import (
	"context"
	"io/fs"
	"sort"
	"sync/atomic"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/promise"

	"github.com/Yiling-J/theine-go"
	"go.uber.org/zap"
)

type routesResult struct {
	err          error
	routes       []*protocol.TunnelRoute
	refreshAfter time.Time
}

const (
	routeCacheBytes  = 1 << 20 // 1MiB
	routePositiveTTL = time.Minute * 5
	routeNegativeTTL = time.Second * 15
	routeFailedTTL   = time.Second * 5
)

func (s *Server) initRouteCache() {
	routeCache, err := theine.NewBuilder[string, *routesResult](routeCacheBytes).Build()

	if err != nil {
		panic("BUG: " + err.Error())
	}

	s.routeCache = routeCache
}

func (s *Server) RoutesPreload(hostname string) {
	s.lookupRoutes(s.ParentContext, hostname, nil)
}

// A stale result requests one refresh, unless another caller already replaced it
// or the most recent refresh is still in its cooldown. Keep results immutable so
// slow callers can identify the exact generation they exhausted.
func (s *Server) lookupRoutes(ctx context.Context, hostname string, stale *routesResult) (*routesResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cached := func() (*routesResult, bool) {
		ret, ok := s.routeCache.Get(hostname)
		return ret, ok && (ret != stale || time.Now().Before(ret.refreshAfter))
	}
	if ret, ok := cached(); ok {
		return ret, nil
	}

	result := s.routeLoads.DoChan(hostname, func() (any, error) {
		// Recheck after joining the flight: a delayed caller must not refresh a
		// newer result just because its own connection attempts took longer.
		if ret, ok := cached(); ok {
			return ret, nil
		}
		// The loader has its own timeout. A cancelled waiter must not cancel
		// the shared lookup or poison the cache for subsequent requests.
		loaded := s.routeCacheLoader(s.ParentContext, hostname)
		if stale != nil {
			loaded.Value.refreshAfter = time.Now().Add(routeFailedTTL)
		}
		ret := &loaded.Value
		s.routeCache.SetWithTTL(hostname, ret, loaded.Cost, loaded.TTL)
		return ret, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ParentContext.Done():
		return nil, s.ParentContext.Err()
	case result := <-result:
		return result.Val.(*routesResult), nil
	}
}

func (s *Server) routeCacheLoader(ctx context.Context, hostname string) (ret theine.Loaded[routesResult]) {
	start := time.Now()
	defer func() {
		s.Logger.Debug("Route cache loader invoked",
			zap.String("hostname", hostname),
			zap.Duration("duration", time.Since(start)),
			zap.Bool("error", ret.Value.err != nil),
			zap.Int64("cost", ret.Cost),
			zap.Duration("ttl", ret.TTL),
		)
	}()

	if home, inv, ok := tun.ParseEphemeralLabel(hostname); ok {
		return s.ephemeralRouteLoader(ctx, hostname, home, inv)
	}

	var (
		numNotFound = 0
		numError    = 0
		numLookup   = tun.NumRedundantLinks
		lookupJobs  = make([]func(context.Context) (*protocol.TunnelRoute, error), tun.NumRedundantLinks)
	)

	for i := range lookupJobs {
		k := i + 1
		lookupJobs[i] = func(ctx context.Context) (*protocol.TunnelRoute, error) {
			key := tun.RoutingKey(hostname, k)
			val, err := s.Chord.Get(ctx, []byte(key))
			if err != nil {
				return nil, err
			}
			if len(val) == 0 {
				return nil, fs.ErrNotExist
			}
			route := &protocol.TunnelRoute{}
			if err := route.UnmarshalVT(val); err != nil {
				return nil, err
			}
			atomic.AddInt64(&ret.Cost, int64(len(val)))
			return route, nil
		}
	}

	lookupCtx, lookupCancel := context.WithTimeout(ctx, lookupTimeout)
	defer lookupCancel()

	routes, errors := promise.All(lookupCtx, lookupJobs...)
	for _, err := range errors {
		switch err {
		case nil:
		case fs.ErrNotExist:
			numNotFound++
		default:
			numError++
		}
	}

	if numLookup == numNotFound {
		ret.Value.err = tun.ErrDestinationNotFound
		ret.TTL = routeNegativeTTL // cache negative result with shorter ttl
		ret.Cost = 8               // use a (1 pointer) cost for negative result
		return
	}

	if numLookup == numNotFound+numError {
		ret.Value.err = tun.ErrLookupFailed
		ret.TTL = routeFailedTTL // also cache failed result with an even shorter ttl
		ret.Cost = 16            // use a (2 pointers) cost for failed result
		return
	}

	// we don't know which one error'd, need to filter nil routes
	filtered := routes[:0]
	for _, route := range routes {
		if route != nil {
			filtered = append(filtered, route)
		}
	}
	// need to nil the elements for gc, if any
	// see https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
	for i := len(filtered); i < len(routes); i++ {
		routes[i] = nil
	}

	// prioritize directly connected route
	localAddress := s.TunnelTransport.Identity().GetAddress()
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].GetTunnelDestination().GetAddress() == localAddress &&
			filtered[j].GetTunnelDestination().GetAddress() != localAddress
	})

	// now we can store the routes on a longer ttl
	ret.Value.routes = filtered
	ret.TTL = routePositiveTTL
	// cost is added atomically during lookup

	return
}

func (s *Server) ephemeralRouteLoader(ctx context.Context, label string, home uint64, inv [16]byte) (ret theine.Loaded[routesResult]) {
	ret.Cost, ret.TTL, ret.Value.err = 256, routeFailedTTL, tun.ErrLookupFailed
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	var dst *protocol.TunnelDestination
	if home == s.Chord.ID() {
		dst = &protocol.TunnelDestination{
			Chord:  s.ChordTransport.Identity(),
			Tunnel: s.TunnelTransport.Identity(),
		}
	} else {
		select {
		case s.ephemeralLoads <- struct{}{}:
		default:
			return
		}
		type result struct {
			node chord.VNode
			err  error
		}
		done := make(chan result, 1)
		go func() {
			defer func() { <-s.ephemeralLoads }()
			node, err := s.Chord.FindSuccessor(home)
			done <- result{node, err}
		}()
		select {
		case <-ctx.Done():
			return
		case found := <-done:
			if found.err != nil {
				return
			}
			if found.node == nil || found.node.ID() != home {
				ret.TTL, ret.Value.err = routeNegativeTTL, tun.ErrDestinationNotFound
				return
			}
			var err error
			dst, err = s.lookupDestination(ctx, tun.DestinationByChordKey(found.node.Identity()))
			if err != nil {
				return
			}
		}
	}
	route := &protocol.TunnelRoute{
		ClientDestination: &protocol.Node{
			Address:    tun.SessionAlias(inv),
			Rendezvous: true,
		},
		ChordDestination:  dst.GetChord(),
		TunnelDestination: dst.GetTunnel(),
		Hostname:          label,
	}
	ret.Cost += int64(route.SizeVT())
	ret.Value.routes, ret.Value.err, ret.TTL = []*protocol.TunnelRoute{route}, nil, routePositiveTTL
	return
}
