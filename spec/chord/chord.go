package chord

import (
	"math/rand"

	"github.com/orisano/wyhash"
)

const (
	// Also known as m in the original paper
	MaxFingerEntries = 42
	// Also known as L in the second paper
	ExtendedSuccessorEntries = 3

	MaxIdentitifer uint64 = 1 << MaxFingerEntries
)

// func init() {
// 	rand.Seed(time.Now().Unix())
// }

func Hash(b []byte) uint64 {
	return wyhash.Sum64(ExtendedSuccessorEntries, b) % MaxIdentitifer
}

func ModuloSum(x, y uint64) uint64 {
	// split (x + y) % m into (x % m + y % m) % m to avoid overflow
	return (x%MaxIdentitifer + y%MaxIdentitifer) % MaxIdentitifer
}

func Random() uint64 {
	return rand.Uint64() % MaxIdentitifer
}

// target IN [low, high)
func BetweenInclusiveLow(low, target, high uint64) bool {
	if high > low {
		return (low <= target && target < high)
	} else {
		return (low <= target || target < high)
	}
}

// target IN (low, high]
func BetweenInclusiveHigh(low, target, high uint64) bool {
	if high > low {
		return (low < target && target <= high)
	} else {
		return (low < target || target <= high)
	}
}

// target IN (low, high)
func BetweenStrict(low, target, high uint64) bool {
	if high > low {
		return low < target && target < high
	} else {
		return low < target || target < high
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
