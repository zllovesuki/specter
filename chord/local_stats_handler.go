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

func (n *LocalNode) printSummary(w http.ResponseWriter) {
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

	fmt.Fprintf(w, "Current state: %s\n", n.state.Get().String())
	fmt.Fprintf(w, "State history: %v\n", n.state.History())
	fmt.Fprintf(w, "---\n")
	fmt.Fprintf(w, "Chord RPC qps: %.2f\n", n.chordRate.RatePerInterval())
	fmt.Fprintf(w, "   KV RPC qps: %.2f\n", n.kvRate.RatePerInterval())
	fmt.Fprintf(w, " Outbound qps: %.2f\n", n.ChordClient.RatePer(time.Second))
	fmt.Fprintf(w, "---\n")

	nodesTable := tablewriter.NewWriter(w)
	nodesTable.SetHeader([]string{"Where", "ID", "Address", "RTT (-10s)"})
	nodesTable.Append([]string{
		"Predecessor",
		fmt.Sprintf("%v", pre.ID()),
		pre.Identity().GetAddress(),
		n.NodesRTT.Snapshot(rtt.MakeMeasurementKey(pre.Identity()), time.Second*10).String(),
	})
	nodesTable.Append([]string{
		"Local",
		fmt.Sprintf("%v", n.ID()),
		n.Identity().GetAddress(),
		n.NodesRTT.Snapshot(rtt.MakeMeasurementKey(n.Identity()), time.Second*10).String(),
	})

	for _, succ := range succList {
		nodesTable.Append([]string{
			fmt.Sprintf("Successor (L = %d)", chord.ExtendedSuccessorEntries),
			fmt.Sprintf("%v", succ.ID()),
			succ.Identity().GetAddress(),
			n.NodesRTT.Snapshot(rtt.MakeMeasurementKey(succ.Identity()), time.Second*10).String(),
		})
	}
	nodesTable.SetCaption(true, fmt.Sprintf("(Last stablized: %s)", n.lastStabilized.Load().Round(time.Second).String()))
	nodesTable.SetAutoMergeCells(true)
	nodesTable.SetRowLine(true)
	nodesTable.Render()

	fmt.Fprintf(w, "---\n")

	finger := n.fingerTrace()
	fingerTable := tablewriter.NewWriter(w)
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

	fmt.Fprintf(w, "---\n")

	keysTable := tablewriter.NewWriter(w)
	keysTable.SetHeader([]string{"owner", "hash(key)", "key", "simple", "prefix", "lease"})

	keys := n.kv.RangeKeys(0, 0)
	exp := n.kv.Export(keys)
	for i, key := range keys {
		plain := exp[i].GetSimpleValue()
		children := exp[i].GetPrefixChildren()
		lease := exp[i].GetLeaseToken()

		id := chord.Hash(key)
		ownership := ""
		if !chord.Between(pre.ID(), id, n.ID(), true) {
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
}

func (n *LocalNode) printKey(w http.ResponseWriter, r *http.Request, key string) {
	val, err := n.kv.Get(r.Context(), []byte(key))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error getting kv: %v", err)
		return
	}
	w.Write(val)
}

func (n *LocalNode) StatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/plain; charset=utf-8")

	query := r.URL.Query()
	if query.Has("key") {
		n.printKey(w, r, query.Get("key"))
		return
	}
	n.printSummary(w)
}
