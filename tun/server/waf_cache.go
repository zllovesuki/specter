package server

import (
	"context"
	"time"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/Yiling-J/theine-go"
)

const (
	hostWAFCacheBytes = 1 << 20 // 1MiB
	hostWAFCacheTTL   = 30 * time.Second
)

type hostWAFCacheEntry struct {
	record    *protocol.HostWAFRecord
	expiresAt time.Time
}

func (s *Server) initHostWAFCache() {
	cache, err := theine.NewBuilder[string, *hostWAFCacheEntry](hostWAFCacheBytes).Build()
	if err != nil {
		panic("BUG: " + err.Error())
	}
	s.hostWAFCache = cache
}

func (s *Server) readCachedHostWAF(ctx context.Context, hostname string) (*protocol.HostWAFRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cached := func() (*hostWAFCacheEntry, bool) {
		entry, ok := s.hostWAFCache.Get(hostname)
		return entry, ok && time.Now().Before(entry.expiresAt)
	}
	if entry, ok := cached(); ok {
		return entry.record, nil
	}

	result := s.hostWAFLoads.DoChan(hostname, func() (any, error) {
		if entry, ok := cached(); ok {
			return entry, nil
		}
		// Invalidating a cache entry can race this read. Preserve its original
		// deadline so an older in-flight read cannot earn another full lifetime.
		expiresAt := time.Now().Add(hostWAFCacheTTL)
		lookupCtx, cancel := context.WithTimeout(s.ParentContext, lookupTimeout)
		defer cancel()
		record, _, err := s.readHostWAF(lookupCtx, hostname)
		if err != nil {
			return nil, err
		}
		if err := lookupCtx.Err(); err != nil {
			return nil, err
		}
		entry := &hostWAFCacheEntry{record: record, expiresAt: expiresAt}
		// Include keys and entry/message overhead, including successful absence.
		cost := int64(256 + len(hostname))
		if record != nil {
			cost += int64(record.SizeVT() + 256 + 64*len(record.Policy.Exclusions))
		}
		if ttl := time.Until(expiresAt); ttl > 0 {
			s.hostWAFCache.SetWithTTL(hostname, entry, cost, ttl)
		}
		return entry, nil
	})
	// A cancelled waiter must not cancel the shared read or cache an error.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ParentContext.Done():
		return nil, s.ParentContext.Err()
	case result := <-result:
		if result.Err != nil {
			return nil, result.Err
		}
		entry := result.Val.(*hostWAFCacheEntry)
		if !time.Now().Before(entry.expiresAt) {
			return nil, context.DeadlineExceeded
		}
		return entry.record, nil
	}
}
