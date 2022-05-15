package node

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"specter/spec/chord"
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

func (n *LocalNode) FingerTrace() string {
	var sb strings.Builder

	ftMap := map[uint64][]int{}
	n.fingerRange(func(i int, f chord.VNode) bool {
		id := f.ID()
		if _, found := ftMap[id]; !found {
			ftMap[id] = make([]int, 0)
		}
		ftMap[id] = append(ftMap[id], i)
		return true
	})

	keys := make([]uint64, 0, len(ftMap))
	for k := range ftMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	for _, k := range keys {
		sb.WriteString(strconv.FormatUint(k, 10))
		sb.WriteString(": ")
		min, max := minmax(ftMap[k])
		sb.WriteString(fmt.Sprintf("%d/%d", min, max))
		sb.WriteString(", ")
	}

	return sb.String()
}

func (n *LocalNode) RingTrace() string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(n.ID(), 10))

	var err error
	var next chord.VNode = n
	seen := make(map[uint64]bool)

	for {
		next, err = n.FindSuccessor(chord.Modulo(next.ID(), 1))
		if err != nil {
			sb.WriteString(" -> ")
			sb.WriteString("error")
			break
		}
		if next == nil {
			break
		}
		if next.ID() == n.ID() {
			sb.WriteString(" -> ")
			sb.WriteString(strconv.FormatUint(n.ID(), 10))
			break
		}
		if seen[next.ID()] {
			return "unstable"
		}
		sb.WriteString(" -> ")
		sb.WriteString(strconv.FormatUint(next.ID(), 10))
		seen[next.ID()] = true
	}

	return sb.String()
}
