package aof

import "time"

func (d *DiskKV) Acquire(lease []byte, ttl time.Duration) (token uint64, err error) {
	return d.memKv.Acquire(lease, ttl)
}

func (d *DiskKV) Renew(lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	return d.memKv.Renew(lease, ttl, prevToken)
}

func (d *DiskKV) Release(lease []byte, token uint64) error {
	return d.memKv.Release(lease, token)
}
