package chord

import (
	"hash/fnv"
	"math/rand"
)

const (
	MaxFingerEntries         int    = 42 // Also known as m in the original paper
	ExtendedSuccessorEntries int    = 4  // Also known as L in the second paper
	MaxIdentitifer           uint64 = 1 << MaxFingerEntries
)

func Hash(b []byte) uint64 {
	h := fnv.New64a()
	h.Write(b)
	// keep lower M-bits of the avalanched, which is the same as modulo
	return h.Sum64() & (MaxIdentitifer - 1)
}

func ModuloSum(x, y uint64) uint64 {
	// split (x + y) % m into (x % m + y % m) % m to avoid overflow
	return (x%MaxIdentitifer + y%MaxIdentitifer) % MaxIdentitifer
}

func Random() uint64 {
	return rand.Uint64() >> (64 - MaxFingerEntries)
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
