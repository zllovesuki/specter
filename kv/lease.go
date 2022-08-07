package kv

import (
	"time"

	"kon.nect.sh/specter/spec/chord"
)

func (m *MemoryMap) Acquire(lease []byte, ttl time.Duration) (token uint64, err error) {
	v, _ := m.fetchVal(lease)
	curr := v.lease.Load()
	ref := time.Now()
	if curr > uint64(ref.UnixNano()) {
		return 0, chord.ErrKVLeaseConflict
	}
	next := uint64(ref.Add(ttl).UnixNano())
	if !v.lease.CompareAndSwap(curr, next) {
		return 0, chord.ErrKVLeaseConflict
	}
	return next, nil
}

func (m *MemoryMap) Renew(lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	v, _ := m.fetchVal(lease)
	curr := v.lease.Load()
	if curr != prevToken {
		return 0, chord.ErrKVLeaseExpired
	}
	new := uint64(time.Now().Add(ttl).UnixNano())
	if !v.lease.CompareAndSwap(curr, new) {
		return 0, chord.ErrKVLeaseExpired
	}
	return new, nil
}

func (m *MemoryMap) Release(lease []byte, token uint64) error {
	v, _ := m.fetchVal(lease)
	if !v.lease.CompareAndSwap(token, 0) {
		return chord.ErrKVLeaseExpired
	}
	return nil
}
