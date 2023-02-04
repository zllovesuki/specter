package chord

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/rtt"

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
	n.fingerRangeView(func(i int, f chord.VNode) bool {
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

func printSummary(w http.ResponseWriter, virtualNodes []*LocalNode) {
	rootNode := virtualNodes[0]

	fmt.Fprintf(w, "Physical node overview:\n")
	fmt.Fprintf(w, "             Address: %s\n", rootNode.Identity().GetAddress())
	fmt.Fprintf(w, "    Outbound RPC qps: %.2f\n", rootNode.ChordClient.RatePer(time.Second))
	fmt.Fprintf(w, "---\n")

	fmt.Fprintf(w, "Virtual nodes overview:\n")
	nodesTable := tablewriter.NewWriter(w)
	nodesTable.SetHeader([]string{"Kind", "ID", "State", "History", "Stablized", "Chord RPC QPS", "KV RPC QPS"})
	for i, node := range virtualNodes {
		kind := "Root"
		if i != 0 {
			kind = "Virtual"
		}
		nodesTable.Append([]string{
			kind,
			fmt.Sprintf("%d", node.ID()),
			node.state.Get().String(),
			fmt.Sprintf("%v", node.state.History()),
			node.lastStabilized.Load().Round(time.Second).String(),
			fmt.Sprintf("%.2f", node.chordRate.RatePerInterval()),
			fmt.Sprintf("%.2f", node.kvRate.RatePerInterval()),
		})
	}
	nodesTable.SetAutoMergeCells(true)
	nodesTable.SetRowLine(true)
	nodesTable.Render()

	fmt.Fprintf(w, "---\n\n")

	for _, node := range virtualNodes {
		fmt.Fprintf(w, "Infomation for virtual node %d\n", node.ID())
		pre, err := node.GetPredecessor()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "error getting predecessor: %v", err)
			return
		}
		succList, err := node.GetSuccessors()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "error getting successor list: %v", err)
			return
		}

		sbs := tablewriter.NewWriter(w)

		nodesString := &strings.Builder{}
		nodesTable := tablewriter.NewWriter(nodesString)
		nodesTable.SetHeader([]string{"Position", "ID", "Address", "RTT (-10s)"})
		nodesTable.Append([]string{
			"Predecessor",
			fmt.Sprintf("%v", pre.ID()),
			pre.Identity().GetAddress(),
			node.NodesRTT.Snapshot(rtt.MakeMeasurementKey(pre.Identity()), time.Second*10).String(),
		})
		nodesTable.Append([]string{
			"Local",
			fmt.Sprintf("%v", node.ID()),
			node.Identity().GetAddress(),
			node.NodesRTT.Snapshot(rtt.MakeMeasurementKey(node.Identity()), time.Second*10).String(),
		})

		for _, succ := range succList {
			nodesTable.Append([]string{
				fmt.Sprintf("Successor (L = %d)", chord.ExtendedSuccessorEntries),
				fmt.Sprintf("%v", succ.ID()),
				succ.Identity().GetAddress(),
				node.NodesRTT.Snapshot(rtt.MakeMeasurementKey(succ.Identity()), time.Second*10).String(),
			})
		}
		nodesTable.SetAutoMergeCells(true)
		nodesTable.SetRowLine(true)
		nodesTable.Render()

		fingerString := &strings.Builder{}
		finger := node.fingerTrace()
		fingerTable := tablewriter.NewWriter(fingerString)
		fingerTable.SetHeader([]string{"Range", "ID"})
		rows := make([][]string, 0)
		for r, id := range finger {
			rows = append(rows, []string{r, id})
		}
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i][0], rows[j][0]
			aParts, bParts := strings.Split(a, "/"), strings.Split(b, "/")
			aMin, _ := strconv.ParseInt(aParts[0], 10, 64)
			bMin, _ := strconv.ParseInt(bParts[0], 10, 64)
			return aMin < bMin
		})
		fingerTable.SetCaption(true, fmt.Sprintf("(range: %v)", chord.MaxIdentitifer))
		fingerTable.SetAutoMergeCells(true)
		fingerTable.SetRowLine(true)
		fingerTable.AppendBulk(rows)
		fingerTable.Render()

		sbs.SetHeader([]string{"Finger", "Ring"})
		sbs.SetAutoWrapText(false)
		sbs.SetAutoFormatHeaders(true)
		sbs.SetCenterSeparator("")
		sbs.SetColumnSeparator("")
		sbs.SetAlignment(tablewriter.ALIGN_CENTER)
		sbs.SetRowSeparator("")
		sbs.SetHeaderLine(false)
		sbs.SetBorder(false)
		sbs.SetTablePadding("\t") // pad with tabs
		sbs.SetNoWhiteSpace(true)
		sbs.Append([]string{fingerString.String(), nodesString.String()})
		sbs.Render()

		keysTable := tablewriter.NewWriter(w)
		keysTable.SetHeader([]string{"owner", "hash(key)", "key", "simple", "prefix", "lease"})

		keys := node.kv.RangeKeys(0, 0)
		exp := node.kv.Export(keys)
		for i, key := range keys {
			plain := exp[i].GetSimpleValue()
			children := exp[i].GetPrefixChildren()
			lease := exp[i].GetLeaseToken()

			id := chord.Hash(key)
			ownership := ""
			if !chord.Between(pre.ID(), id, node.ID(), true) {
				ownership = "X"
			}
			raw := []any{ownership, id, string(key), len(plain), len(children), lease}
			row := make([]string, len(raw))
			for i, r := range raw {
				row[i] = fmt.Sprintf("%v", r)
			}
			keysTable.Append(row)
		}
		keysTable.SetCaption(true, fmt.Sprintf("(With %d keys; X in owner column indicates incorrect owner)", len(keys)))
		keysTable.SetAutoMergeCells(true)
		keysTable.SetRowLine(true)
		keysTable.Render()

		fmt.Fprintf(w, "---\n\n")
	}
}

func printKey(virtualNodes []*LocalNode, w http.ResponseWriter, r *http.Request, key string) {
	for _, n := range virtualNodes {
		val, err := n.kv.Get(r.Context(), []byte(key))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "error getting kv: %v", err)
			return
		}
		if len(val) != 0 {
			w.Write(val)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, "key not found on this node")
}

func StatsHandler(virtualNodes []*LocalNode) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/plain; charset=utf-8")

		query := r.URL.Query()
		if query.Has("key") {
			printKey(virtualNodes, w, r, query.Get("key"))
			return
		}

		printSummary(w, virtualNodes)
	}
}
