package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/tun"

	"github.com/go-chi/chi/v5"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/twitchtv/twirp"
)

func TunnelServerHandler(s *Server) http.Handler {
	router := chi.NewRouter()

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		clients := s.TunnelTransport.ListConnected()

		clientTable := table.NewWriter()
		clientTable.SetOutputMirror(w)

		clientTable.AppendHeader(table.Row{"Identity", "Address", "Number of tunnels"})
		for _, h := range clients {
			var (
				numTunnels string
				node       = h.Identity
				addr       = h.Addr
			)
			prefix := tun.ClientHostnamesPrefix(&protocol.ClientToken{
				Token: []byte(node.GetAddress()),
			})
			children, err := s.Chord.PrefixList(r.Context(), []byte(prefix))
			if err != nil {
				numTunnels = err.Error()
			} else {
				numTunnels = fmt.Sprintf("%d", len(children))
			}
			clientTable.AppendRow(table.Row{
				fmt.Sprintf("%d/%s", node.GetId(), node.GetAddress()),
				addr.String(),
				numTunnels,
			})
		}

		clientTable.SetStyle(table.StyleDefault)
		clientTable.Style().Options.SeparateRows = true
		clientTable.Render()
	})

	router.Get("/{id}/*", func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		address := chi.URLParam(r, "*")

		client := &protocol.Node{
			Id:         id,
			Address:    address,
			Rendezvous: true,
		}

		// default to http client pooling
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.DisableKeepAlives = true
		t.MaxConnsPerHost = -1
		t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return s.TunnelTransport.DialStream(ctx, client, protocol.Stream_RPC)
		}
		c := &http.Client{
			Transport: t,
		}

		rpcClient := protocol.NewClientQueryServiceProtobufClient("http://client", c)

		callCtx, cancel := context.WithTimeout(r.Context(), lookupTimeout)
		defer cancel()

		resp, err := rpcClient.ListTunnels(callCtx, &protocol.ListTunnelsRequest{})
		if err != nil {
			twirp.WriteError(w, err)
			return
		}

		tunnelMap := make(map[string]string)
		for _, t := range resp.GetTunnels() {
			tunnelMap[t.Hostname] = t.Target
		}

		prefix := tun.ClientHostnamesPrefix(&protocol.ClientToken{
			Token: []byte(client.GetAddress()),
		})
		children, err := s.Chord.PrefixList(callCtx, []byte(prefix))
		if err != nil {
			twirp.WriteError(w, rpc.WrapErrorKV(prefix, err))
			return
		}

		tunnelTable := table.NewWriter()
		tunnelTable.SetOutputMirror(w)

		tunnelTable.AppendHeader(table.Row{"Hostname", "Target"})
		for _, child := range children {
			hostname := string(child)
			target, ok := tunnelMap[hostname]
			if ok {
				tunnelTable.AppendRow(table.Row{hostname, target})
			} else {
				tunnelTable.AppendRow(table.Row{hostname, "(unused)"})
			}
		}

		tunnelTable.SetStyle(table.StyleDefault)
		tunnelTable.Style().Options.SeparateRows = true
		tunnelTable.Render()
	})

	return router
}
