package lease

import (
	"math"
	"time"

	"go.miragespace.co/specter/spec/chord"
)

// Next returns a positive UnixNano expiry that advances the previous token.
// Providers pass zero for acquisition and retain their atomic ownership checks.
func Next(now time.Time, ttl time.Duration, previous uint64) (uint64, error) {
	nanos := now.UnixNano()
	if nanos < 0 || ttl < time.Second || int64(ttl) > math.MaxInt64-nanos {
		return 0, chord.ErrKVLeaseInvalidTTL
	}
	if previous >= math.MaxInt64 {
		return 0, chord.ErrKVLeaseExpired
	}
	return max(uint64(nanos+int64(ttl)), previous+1), nil
}
