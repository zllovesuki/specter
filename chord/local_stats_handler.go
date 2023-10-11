package chord

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/rtt"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
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

	phy := &strings.Builder{}
	vir := &strings.Builder{}
	lis := &strings.Builder{}

	nodesTable := table.NewWriter()
	nodesTable.SetOutputMirror(vir)
	nodesTable.AppendHeader(table.Row{"Kind", "ID", "State", "History", "Stablized", "Chord RPC QPS", "RPC Error", "KV RPC QPS", "Stale KV"})
	nodesTable.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, AutoMerge: true, AlignHeader: text.AlignCenter},
		{Number: 2, AlignHeader: text.AlignCenter},
		{Number: 3, AutoMerge: true, AlignHeader: text.AlignCenter},
		{Number: 4, AlignHeader: text.AlignCenter, WidthMax: 40, WidthMaxEnforcer: text.WrapSoft},
		{Number: 5, AutoMerge: true, AlignHeader: text.AlignCenter},
		{Number: 6, AutoMerge: true, AlignHeader: text.AlignCenter, Align: text.AlignRight},
		{Number: 7, AutoMerge: true, AlignHeader: text.AlignCenter, Align: text.AlignRight},
		{Number: 8, AutoMerge: true, AlignHeader: text.AlignCenter, Align: text.AlignRight},
		{Number: 9, AutoMerge: true, AlignHeader: text.AlignCenter, Align: text.AlignRight},
	})
	for i, node := range virtualNodes {
		kind := "Root"
		if i != 0 {
			kind = "Virtual"
		}
		nodesTable.AppendRow(table.Row{
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
	nodesTable.SetStyle(table.StyleDefault)
	nodesTable.Style().Options.SeparateRows = true
	nodesTable.Render()

	for _, node := range virtualNodes {
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

		sbs := table.NewWriter()
		sbs.SetOutputMirror(lis)

		nodesString := &strings.Builder{}
		nodesTable := table.NewWriter()
		nodesTable.SetOutputMirror(nodesString)
		nodesTable.AppendHeader(table.Row{"Position", "ID", "Address", "RTT (-10s)"})
		nodesTable.SetColumnConfigs([]table.ColumnConfig{
			{Number: 1, AutoMerge: true, AlignHeader: text.AlignCenter},
			{Number: 2, AlignHeader: text.AlignCenter},
			{Number: 3, AutoMerge: true, AlignHeader: text.AlignCenter},
			{Number: 4, AutoMerge: true, AlignHeader: text.AlignCenter},
		})
		nodesTable.AppendRows([]table.Row{
			{
				"Predecessor",
				fmt.Sprintf("%v", pre.ID()),
				pre.Identity().GetAddress(),
				node.NodesRTT.Snapshot(rtt.MakeMeasurementKey(pre.Identity()), time.Second*10).String(),
			},
			{
				"Local",
				fmt.Sprintf("%v", node.ID()),
				node.Identity().GetAddress(),
				node.NodesRTT.Snapshot(rtt.MakeMeasurementKey(node.Identity()), time.Second*10).String(),
			},
		})

		for _, succ := range succList {
			nodesTable.AppendRow(table.Row{
				fmt.Sprintf("Successor (L = %d)", chord.ExtendedSuccessorEntries),
				fmt.Sprintf("%v", succ.ID()),
				succ.Identity().GetAddress(),
				node.NodesRTT.Snapshot(rtt.MakeMeasurementKey(succ.Identity()), time.Second*10).String(),
			})
		}
		nodesTable.SetStyle(table.StyleDefault)
		nodesTable.Style().Options.SeparateRows = true
		nodesTable.Render()

		fingerString := &strings.Builder{}
		finger := node.fingerTrace()
		fingerTable := table.NewWriter()
		fingerTable.SetOutputMirror(fingerString)
		fingerTable.AppendHeader(table.Row{"Range", "ID"})
		rows := make([]table.Row, 0)
		for r, id := range finger {
			rows = append(rows, table.Row{r, id})
		}
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i][0], rows[j][0]
			aParts, bParts := strings.Split(a.(string), "/"), strings.Split(b.(string), "/")
			aMin, _ := strconv.ParseInt(aParts[0], 10, 64)
			bMin, _ := strconv.ParseInt(bParts[0], 10, 64)
			return aMin < bMin
		})
		fingerTable.AppendRows(rows, table.RowConfig{AutoMerge: true})
		fingerTable.SetCaption("(range: %v)", chord.MaxIdentitifer)
		fingerTable.SetStyle(table.StyleDefault)
		fingerTable.Style().Options.SeparateRows = true
		fingerTable.SetColumnConfigs([]table.ColumnConfig{
			{Number: 1, AlignHeader: text.AlignCenter},
			{Number: 2, AlignHeader: text.AlignCenter},
		})
		fingerTable.Render()

		sbs.AppendHeader(table.Row{"Finger", "Ring"})
		sbs.SetColumnConfigs([]table.ColumnConfig{
			{Number: 1, AlignHeader: text.AlignCenter},
			{Number: 2, AlignHeader: text.AlignCenter},
		})
		sbs.AppendRow(table.Row{fingerString.String(), nodesString.String()})
		sbs.SetStyle(table.StyleDefault)
		sbs.Style().Options.DrawBorder = false
		sbs.Style().Options.SeparateHeader = false
		sbs.Style().Options.SeparateColumns = false
		sbs.Style().Options.SeparateRows = false
		sbs.Render()

		keysTable := table.NewWriter()
		keysTable.SetOutputMirror(lis)
		keysTable.AppendHeader(table.Row{"owner", "hash(key)", "key", "simple", "prefix", "lease"})
		keysTable.SetColumnConfigs([]table.ColumnConfig{
			{Number: 1, AutoMerge: true, AlignHeader: text.AlignCenter},
			{Number: 2, AlignHeader: text.AlignCenter},
			{Number: 3, AlignHeader: text.AlignCenter, WidthMin: 20},
			{Number: 4, AutoMerge: true, AlignHeader: text.AlignCenter},
			{Number: 5, AutoMerge: true, AlignHeader: text.AlignCenter},
			{Number: 6, AutoMerge: true, AlignHeader: text.AlignCenter},
		})

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
			keysTable.AppendRow(table.Row{
				ownership, id, string(key), len(plain), len(children), lease,
			})
		}
		numKeys += len(keys)
		keysTable.SetCaption("(With %d keys; X in owner column indicates incorrect owner)", len(keys))
		keysTable.SetStyle(table.StyleDefault)
		keysTable.Style().Options.SeparateRows = true
		keysTable.SuppressEmptyColumns()
		keysTable.Render()

		fmt.Fprintf(lis, "---\n\n")
	}

	// read mem stats
	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)
	phyTable := table.NewWriter()
	phyTable.SetOutputMirror(phy)
	phyTable.AppendRows([]table.Row{
		{
			"Advertise Address",
			rootNode.Identity().GetAddress(),
		},
		{
			"Outbound RPC qps",
			fmt.Sprintf("%.2f", rootNode.ChordClient.RatePer(time.Second)),
		},
		{
			"Number of KV Keys",
			fmt.Sprintf("%d", numKeys),
		},
		{"Mallocs", fmt.Sprintf("%d", rtm.Alloc)},
		{"Frees", fmt.Sprintf("%d", rtm.Frees)},
		{"LiveObjects", fmt.Sprintf("%d", rtm.Mallocs-rtm.Frees)},
		{"HeapObjects", fmt.Sprintf("%d", rtm.HeapObjects)},
		{"HeapAlloc", fmt.Sprintf("%d", rtm.HeapAlloc)},
		{"NumGC", fmt.Sprintf("%d", rtm.NumGC)},
		{"PauseTotalNs", fmt.Sprintf("%d", rtm.PauseTotalNs)},
		{"LastGC", time.UnixMilli(int64(rtm.LastGC / 1_000_000)).Format(time.RFC3339)},
	})
	phyTable.SetStyle(table.StyleDefault)
	phyTable.Style().Options.SeparateRows = true
	phyTable.SetColumnConfigs([]table.ColumnConfig{
		{Number: 2, Align: text.AlignRight},
	})
	phyTable.Render()

	w.Write([]byte(phy.String()))
	fmt.Fprintf(w, "---\n\n")
	w.Write([]byte(vir.String()))
	fmt.Fprintf(w, "---\n\n")
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

func statsHandler(virtualNodes []*LocalNode) func(w http.ResponseWriter, r *http.Request) {
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
