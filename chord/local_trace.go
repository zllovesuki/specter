package chord

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

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

func (n *LocalNode) fingerTrace() string {
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
		min, max := minmax(ftMap[k])
		sb.WriteString(fmt.Sprintf("%d/%d", min, max))
		sb.WriteString(": ")
		sb.WriteString(strconv.FormatUint(k, 10))
		sb.WriteString(", ")
	}

	return sb.String()
}

func (n *LocalNode) ringTrace() string {
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

func (n *LocalNode) StatsHandler(w http.ResponseWriter, r *http.Request) {
	pre, err := n.GetPredecessor()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error getting predecessor: %v", err)
	}
	succList, err := n.GetSuccessors()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error getting successor list: %v", err)
	}

	w.Header().Set("content-type", "text/plain")
	fmt.Fprintf(w, "Predecessor: %d - %s\n", pre.ID(), pre.Identity().GetAddress())
	fmt.Fprintf(w, "LocalNode: %d - %s\n", n.ID(), n.Identity().GetAddress())
	fmt.Fprintf(w, "Successors: \n")
	for _, succ := range succList {
		fmt.Fprintf(w, "  %15d - %s\n", succ.ID(), succ.Identity().GetAddress())
	}
	fmt.Fprintf(w, "---\n")
	finger := n.fingerTrace()
	fmt.Fprintf(w, "FingerTable: \n%s\n", finger)
	keys := n.kv.RangeKeys(0, 0)
	fmt.Fprintf(w, "---\n")
	fmt.Fprintf(w, "keys on current node:\n")
	for _, key := range keys {
		fmt.Fprintf(w, "  %15d - %s\n", chord.Hash(key), key)
	}
}
