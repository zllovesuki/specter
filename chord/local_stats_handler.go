package chord

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/rtt"

	"github.com/go-chi/chi/v5/middleware"
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
	numKeys := 0

	vir := &strings.Builder{}
	fmt.Fprintf(vir, "== Virtual nodes overview ==\n")

	nodesTable := tablewriter.NewWriter(vir)
	nodesTable.SetHeader([]string{"Kind", "ID", "State", "History", "Stablized", "Chord RPC QPS", "RPC Error", "KV RPC QPS", "Stale KV"})
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
			node.lastStabilized.Load().Round(time.Second).Format(time.RFC3339),
			fmt.Sprintf("%.2f", node.chordRate.RatePerInterval()),
			fmt.Sprintf("%d", node.rpcErrorCount.Load()),
			fmt.Sprintf("%.2f", node.kvRate.RatePerInterval()),
			fmt.Sprintf("%d", node.kvStaleCount.Load()),
		})
	}
	nodesTable.SetAutoMergeCells(true)
	nodesTable.SetRowLine(true)
	nodesTable.Render()

	fmt.Fprintf(vir, "---\n\n")

	lis := &strings.Builder{}
	for _, node := range virtualNodes {
		fmt.Fprintf(lis, "== Infomation for virtual node %d ==\n", node.ID())
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

		sbs := tablewriter.NewWriter(lis)

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

		keysTable := tablewriter.NewWriter(lis)
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
		numKeys += len(keys)
		keysTable.SetCaption(true, fmt.Sprintf("(With %d keys; X in owner column indicates incorrect owner)", len(keys)))
		keysTable.SetAutoMergeCells(true)
		keysTable.SetRowLine(true)
		keysTable.Render()

		fmt.Fprintf(lis, "---\n\n")
	}

	phy := &strings.Builder{}
	fmt.Fprintf(phy, "== Physical node overview ==\n")
	phyTable := tablewriter.NewWriter(phy)
	phyTable.Append([]string{
		"Advertise Address",
		rootNode.Identity().GetAddress(),
	})
	phyTable.Append([]string{
		"Outbound RPC qps",
		fmt.Sprintf("%.2f", rootNode.ChordClient.RatePer(time.Second)),
	})
	phyTable.Append([]string{
		"Number of KV Keys",
		fmt.Sprintf("%d", numKeys),
	})
	// read mem stats
	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)
	phyTable.AppendBulk([][]string{
		{"Mallocs", fmt.Sprintf("%d", rtm.Alloc)},
		{"Frees", fmt.Sprintf("%d", rtm.Frees)},
		{"LiveObjects", fmt.Sprintf("%d", rtm.Mallocs-rtm.Frees)},
		{"HeapObjects", fmt.Sprintf("%d", rtm.HeapObjects)},
		{"HeapAlloc", fmt.Sprintf("%d", rtm.HeapAlloc)},
		{"NumGC", fmt.Sprintf("%d", rtm.NumGC)},
		{"PauseTotalNs", fmt.Sprintf("%d", rtm.PauseTotalNs)},
		{"LastGC", time.UnixMilli(int64(rtm.LastGC / 1_000_000)).Format(time.RFC3339)},
	})
	phyTable.SetAlignment(tablewriter.ALIGN_RIGHT)
	phyTable.SetAutoWrapText(false)
	phyTable.SetHeaderLine(false)
	phyTable.SetRowLine(true)
	phyTable.Render()
	fmt.Fprintf(phy, "---\n\n")

	w.Write([]byte(phy.String()))
	w.Write([]byte(vir.String()))
	w.Write([]byte(lis.String()))
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
		query := r.URL.Query()
		if query.Has("key") {
			w.Header().Set("content-type", "text/plain; charset=utf-8")
			printKey(virtualNodes, w, r, query.Get("key"))
			return
		}

		urlFormat, _ := r.Context().Value(middleware.URLFormatCtxKey).(string)
		switch urlFormat {
		case "html":
			w.Header().Set("content-type", "text/html; charset=utf-8")
			defer fmt.Fprint(w, `</pre></body></html>`)
			fmt.Fprint(w, `<html><head><meta name="viewport" content="width=device-width, initial-scale=1.0"></head><body><pre>`)
		default:
			w.Header().Set("content-type", "text/plain; charset=utf-8")
		}

		printSummary(w, virtualNodes)
	}
}
