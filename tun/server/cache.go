package server

import (
	"context"
	"io/fs"
	"sort"
	"sync/atomic"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/promise"

	"github.com/Yiling-J/theine-go"
	"go.uber.org/zap"
)

type routesResult struct {
	err    error
	routes []*protocol.TunnelRoute
}

const (
	cacheBytes  = 1 << 20 // 1MiB
	positiveTTL = time.Minute * 5
	negativeTTL = time.Second * 15
	failedTTL   = time.Second * 5
)

func (s *Server) initRouteCache() {
	routeCache, err := theine.NewBuilder[string, routesResult](cacheBytes).
		// listen for cache entry removal
		// TODO: metrics
		RemovalListener(s.cacheEventListener).
		// configure loader to fetch routes on miss
		// TODO: make routing selection more intelligent with rtt
		BuildWithLoader(s.cacheLoader)

	if err != nil {
		panic("BUG: " + err.Error())
	}

	s.routeCache = routeCache
}

func (s *Server) RoutesPreload(hostname string) {
	s.routeCache.Get(s.ParentContext, hostname)
}

func (s *Server) cacheEventListener(hostname string, ret routesResult, reason theine.RemoveReason) {
	var reasonStr string
	switch reason {
	case theine.EVICTED:
		reasonStr = "evicted"
	case theine.EXPIRED:
		reasonStr = "expired"
	case theine.REMOVED:
		reasonStr = "removed"
	default:
		reasonStr = "unknown"
	}
	s.Logger.Debug("Route cache entry removed", zap.String("hostname", hostname), zap.String("reason", reasonStr))
}

func (s *Server) cacheLoader(ctx context.Context, hostname string) (ret theine.Loaded[routesResult], loadErr error) {
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
		ret.TTL = negativeTTL // cache negative result with shorter ttl
		ret.Cost = 8          // use a (1 pointer) cost for negative result
		return
	}

	if numLookup == numError {
		ret.Value.err = tun.ErrLookupFailed
		ret.TTL = failedTTL // also cache failed result with an even shorter ttl
		ret.Cost = 16       // use a (2 pointers) cost for failed result
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
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].GetTunnelDestination().GetAddress() == s.TunnelTransport.Identity().GetAddress()
	})

	// now we can store the routes on a longer ttl
	ret.Value.routes = filtered
	ret.TTL = positiveTTL
	// cost is added atomically during lookup

	return
}
