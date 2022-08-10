package chord

import (
	"fmt"
	"net/http"
	"time"

	"kon.nect.sh/specter/spec/chord"

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

func (n *LocalNode) printKey(w http.ResponseWriter, key string) {
	val, err := n.kv.Get([]byte(key))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error getting kv: %v", err)
		return
	}
	w.Write(val)
}

func (n *LocalNode) StatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/plain")

	query := r.URL.Query()
	if query.Has("key") {
		n.printKey(w, query.Get("key"))
		return
	}
	n.printSummary(w)
}
