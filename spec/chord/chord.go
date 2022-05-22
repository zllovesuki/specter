package chord

import (
	"math/rand"
	"time"

	"github.com/zeebo/xxh3"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	// Also known as m in the original paper
	MaxFingerEntries = 48
	// Also known as L in the second paper
	ExtendedSuccessorEntries = 3

	mod = (1 << MaxFingerEntries)
)

func Hash(b []byte) uint64 {
	hasher := xxh3.New()
	hasher.Write(b)
	return hasher.Sum64() % mod
}

func HashString(key string) uint64 {
	hasher := xxh3.New()
	hasher.WriteString(key)
	return hasher.Sum64() % mod
}

func Modulo(x, y uint64) uint64 {
	// split (x + y) % m into (x % m + y % m) % m to avoid overflow
	return (x%mod + y%mod) % mod
}

func Random() uint64 {
	return rand.Uint64() % mod
}

func Between(low, target, high uint64, inclusive bool) bool {
	// account for loop around
	if high > low {
		return (low < target && target < high) || (inclusive && target == high)
	} else {
		return low < target || target < high || (inclusive && target == high)
	}
}
