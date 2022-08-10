package chord

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/chord"

	"github.com/olekukonko/tablewriter"
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

	f := make(map[string]string)
	for _, k := range keys {
		min, max := minmax(ftMap[k])
		f[fmt.Sprintf("%d/%d", min, max)] = strconv.FormatUint(k, 10)
	}

	return f
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
		return
	}
	succList, err := n.GetSuccessors()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error getting successor list: %v", err)
		return
	}
	w.Header().Set("content-type", "text/plain")

	fmt.Fprintf(w, "Current state: %s\n", n.state.String())
	fmt.Fprintf(w, "---\n")

	nodesTable := tablewriter.NewWriter(w)
	nodesTable.SetHeader([]string{"Where", "ID", "Address"})
	nodesTable.Append([]string{"Precedessor", fmt.Sprintf("%v", pre.ID()), pre.Identity().GetAddress()})
	nodesTable.Append([]string{"Local", fmt.Sprintf("%v", n.ID()), n.Identity().GetAddress()})

	for _, succ := range succList {
		nodesTable.Append([]string{fmt.Sprintf("(Successor; L = %d)", chord.ExtendedSuccessorEntries), fmt.Sprintf("%v", succ.ID()), succ.Identity().GetAddress()})
	}
	nodesTable.SetCaption(true, fmt.Sprintf("(stablized: %s)", n.lastStabilized.Load().Round(time.Second).String()))
	nodesTable.SetAutoMergeCells(true)
	nodesTable.SetRowLine(true)
	nodesTable.Render()

	fmt.Fprintf(w, "---\n")

	finger := n.fingerTrace()
	fingerTable := tablewriter.NewWriter(w)
	fingerTable.SetHeader([]string{"Range", "ID"})
	for r, id := range finger {
		fingerTable.Append([]string{r, id})
	}
	fingerTable.SetCaption(true, fmt.Sprintf("(range: %v)", uint64(1<<chord.MaxFingerEntries)))
	fingerTable.SetAutoMergeCells(true)
	fingerTable.SetRowLine(true)
	fingerTable.Render()

	fmt.Fprintf(w, "---\n")

	keysTable := tablewriter.NewWriter(w)
	keysTable.SetHeader([]string{"hash(key)", "key", "simple", "prefix", "lease"})

	keys := n.kv.RangeKeys(0, 0)
	exp := n.kv.Export(keys)
	for i, key := range keys {
		plain := exp[i].GetSimpleValue()
		children := exp[i].GetPrefixChildren()
		lease := exp[i].GetLeaseToken()
		raw := []any{
			chord.Hash(key),
			string(key),
			plain != nil,
			len(children),
			lease,
		}
		row := make([]string, len(raw))
		for i, r := range raw {
			row[i] = fmt.Sprintf("%v", r)
		}
		keysTable.Append(row)
	}
	good := kvFsck(n.kv, pre.ID(), n.ID())
	keysTable.SetCaption(true, fmt.Sprintf("(fsck: %v)", good))
	keysTable.SetAutoMergeCells(true)
	keysTable.SetRowLine(true)
	keysTable.Render()
}

func kvFsck(kv chord.KVProvider, low, high uint64) bool {
	valid := true

	keys := kv.RangeKeys(0, 0)
	for _, key := range keys {
		if !chord.Between(low, chord.Hash(key), high, true) {
			valid = false
		}
	}
	return valid
}
