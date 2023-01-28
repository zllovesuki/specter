package chord

import (
	"fmt"
	"sort"
	"strconv"

	"kon.nect.sh/specter/spec/chord"
)

func minmax(nums []int) (min, max int) {
	min = nums[0]
	max = nums[0]
	for _, num := range nums {
		if num > max {
			max = num
		}
		if num < min {
			min = num
		}
	}
	return
}

func (n *LocalNode) fingerTrace() map[string]string {
	var (
		ftMap = map[uint64][]int{}
		entry *fingerEntry
	)
	for k := chord.MaxFingerEntries; k >= 1; k-- {
		entry = &n.fingers[k]
		entry.mu.RLock()
		id := entry.node.ID()
		entry.mu.RUnlock()
		if _, found := ftMap[id]; !found {
			ftMap[id] = make([]int, 0)
		}
		ftMap[id] = append(ftMap[id], k)
	}

	keys := make([]uint64, 0, len(ftMap))
	for k := range ftMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	f := make(map[string]string)
	for _, k := range keys {
		min, max := minmax(ftMap[k])
		f[fmt.Sprintf("%d/%d", min, max)] = strconv.FormatUint(k, 10)
	}

	return f
}

func (n *LocalNode) ringTrace() string {
	return ""
	// var sb strings.Builder
	// sb.WriteString(strconv.FormatUint(n.ID(), 10))

	// var err error
	// var next chord.VNode = n
	// seen := make(map[uint64]bool)

	// for {
	// 	key := chord.ModuloSum(next.ID(), 1)
	// 	n.Logger.Debug("fs req", zap.Uint64("key", key))
	// 	next, err = n.FindSuccessor(chord.ModuloSum(next.ID(), 1))
	// 	n.Logger.Debug("fs resp", zap.Uint64("key", key))
	// 	if err != nil {
	// 		sb.WriteString(" -> ")
	// 		sb.WriteString("error")
	// 		break
	// 	}
	// 	if next == nil {
	// 		break
	// 	}
	// 	if next.ID() == n.ID() {
	// 		sb.WriteString(" -> ")
	// 		sb.WriteString(strconv.FormatUint(n.ID(), 10))
	// 		break
	// 	}
	// 	if seen[next.ID()] {
	// 		return "unstable"
	// 	}
	// 	sb.WriteString(" -> ")
	// 	sb.WriteString(strconv.FormatUint(next.ID(), 10))
	// 	seen[next.ID()] = true
	// }

	// return sb.String()
}
