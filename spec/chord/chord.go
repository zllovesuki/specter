package chord

import (
	"math/rand"

	"github.com/zeebo/xxh3"
)

const (
	// Also known as m in the original paper
	MaxFingerEntries = 48
	// Also known as L in the second paper
	ExtendedSuccessorEntries = 3

	MaxIdentitifer = (1 << (MaxFingerEntries - 1))
)

func Hash(b []byte) uint64 {
	hasher := xxh3.New()
	hasher.Write(b)
	return hasher.Sum64() % MaxIdentitifer
}

func HashString(key string) uint64 {
	hasher := xxh3.New()
	hasher.WriteString(key)
	return hasher.Sum64() % MaxIdentitifer
}

func Modulo(x, y uint64) uint64 {
	// split (x + y) % m into (x % m + y % m) % m to avoid overflow
	return (x%MaxIdentitifer + y%MaxIdentitifer) % MaxIdentitifer
}

func Random() uint64 {
	return rand.Uint64() % MaxIdentitifer
}

func Between(low, target, high uint64, inclusive bool) bool {
	// account for loop around
	if high > low {
		return (low < target && target < high) || (inclusive && target == high)
	} else {
		return low < target || target < high || (inclusive && target == high)
	}
}

// make successor list that will not have duplicate VNodes
func MakeSuccList(immediate VNode, successors []VNode, maxLen int) []VNode {
	succList := []VNode{immediate}
	seen := make(map[uint64]bool)
	seen[immediate.ID()] = true

	for _, succ := range successors {
		if len(succList) >= maxLen {
			break
		}
		if succ == nil || seen[succ.ID()] {
			continue
		}
		seen[succ.ID()] = true
		succList = append(succList, succ)
	}
	return succList
}
