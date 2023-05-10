package chord

import (
	"fmt"
	"net/http"
	"strconv"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
)

func formatNode(n *protocol.Node) string {
	return fmt.Sprintf("%s/%d", n.GetAddress(), n.GetId())
}

var vOptions = []func(*graph.VertexProperties){
	graph.VertexAttribute("shape", "box"),
}

var rootVOptions = append(vOptions,
	graph.VertexAttribute("style", "filled"),
	graph.VertexAttribute("color", "yellow"),
)

var selfVOptions = append(vOptions,
	graph.VertexAttribute("style", "filled"),
	graph.VertexAttribute("color", "lightgrey"),
)

func ringGraphHandler(root *LocalNode) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		var next chord.VNode = root

		nodes := make([]*protocol.Node, 0)
		seen := make(map[uint64]bool)
		lookup := make(map[string]*protocol.Node)

		predecessor := root.getPredecessor()
		if predecessor == nil {
			http.Error(w, "predecessor is nil", 500)
			return
		}

		for {
			next, err = root.FindSuccessor(chord.ModuloSum(next.ID(), 1))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if next == nil {
				http.Error(w, "successor is nil", 500)
				return
			}
			if next.ID() == root.ID() {
				nodes = append(nodes, root.Identity())
				break
			}
			if seen[next.ID()] {
				http.Error(w, "ring is unstable", 500)
				return
			}
			nodes = append(nodes, next.Identity())
			seen[next.ID()] = true
		}

		ring := graph.New(formatNode, graph.Directed())

		for _, node := range nodes {
			if node.GetId() == root.ID() {
				ring.AddVertex(node, rootVOptions...)
			} else if node.GetAddress() == root.Identity().GetAddress() {
				ring.AddVertex(node, selfVOptions...)
			} else {
				ring.AddVertex(node, vOptions...)
			}
			lookup[strconv.FormatUint(node.GetId(), 10)] = node
		}

		ring.AddEdge(formatNode(root.Identity()), formatNode(predecessor.Identity()), graph.EdgeAttribute("color", "blue"))
		for i := 0; i < len(nodes)-1; i++ {
			ring.AddEdge(formatNode(nodes[i]), formatNode(nodes[i+1]), graph.EdgeAttribute("color", "red"))
		}
		ring.AddEdge(formatNode(nodes[len(nodes)-1]), formatNode(nodes[0]), graph.EdgeAttribute("color", "red"))

		fingers := root.fingerTrace()
		for r, id := range fingers {
			s := formatNode(root.Identity())
			d := formatNode(lookup[id])
			err := ring.AddEdge(s, d, graph.EdgeAttribute("label", r))
			if err == graph.ErrEdgeAlreadyExists {
				ring.RemoveEdge(s, d)
				ring.AddEdge(s, d, graph.EdgeAttribute("label", r), graph.EdgeAttribute("color", "red"))
			}
		}

		w.Header().Set("content-type", "text/plain")
		draw.DOT(ring, w)
	}
}
