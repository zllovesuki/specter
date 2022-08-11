package util

import (
	"math/rand"
	"time"
)

// RandomTimeRange returns time.Duration between [interval/2, interval] randomly
func RandomTimeRange(interval time.Duration) time.Duration {
	t := interval / 2
	return time.Duration(rand.Int63n(int64(t*2)-int64(t)) + int64(t))
}
