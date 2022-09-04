package memory

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/chord"
)

var _ chord.LeaseKV = (*MemoryKV)(nil)

func durationGuard(t time.Duration) (time.Duration, bool) {
	td := t.Truncate(time.Second)
	if td < time.Second {
		return 0, false
	}
	return td, true
}

func (m *MemoryKV) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (token uint64, err error) {
	ttl, ok := durationGuard(ttl)
	if !ok {
		return 0, chord.ErrKVLeaseInvalidTTL
	}
	v, _ := m.fetchVal(lease)
	curr := v.lease.Load()
	ref := time.Now() // questionable
	if curr > uint64(ref.UnixNano()) {
		return 0, chord.ErrKVLeaseConflict
	}
	next := uint64(ref.Add(ttl).UnixNano())
	if !v.lease.CompareAndSwap(curr, next) {
		return 0, chord.ErrKVLeaseConflict
	}
	return next, nil
}

func (m *MemoryKV) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	ttl, ok := durationGuard(ttl)
	if !ok {
		return 0, chord.ErrKVLeaseInvalidTTL
	}
	v, _ := m.fetchVal(lease)
	curr := v.lease.Load()
	if curr != prevToken {
		return 0, chord.ErrKVLeaseExpired
	}
	next := uint64(time.Now().Add(ttl).UnixNano())
	if !v.lease.CompareAndSwap(curr, next) {
		return 0, chord.ErrKVLeaseExpired
	}
	return next, nil
}

func (m *MemoryKV) Release(ctx context.Context, lease []byte, token uint64) error {
	v, _ := m.fetchVal(lease)
	if !v.lease.CompareAndSwap(token, 0) {
		return chord.ErrKVLeaseExpired
	}
	return nil
}
