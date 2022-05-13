package chord

import (
	"hash/fnv"
)

const (
	// Also known as M in the original paper
	MaxFingerEntries = 8
)

func Hash(b []byte) uint64 {
	hasher := fnv.New64a()
	hasher.Write(b)
	return hasher.Sum64() % (1 << MaxFingerEntries)
}

func HashString(key string) uint64 {
	return Hash([]byte(key))
}

func Between(low, target, high uint64, inclusive bool) bool {
	// account for loop around
	if high > low {
		return (low < target && target < high) || (inclusive && target == high)
	} else {
		return low < target || target < high || (inclusive && target == high)
	}
}
