package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strconv"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/tun"

	"github.com/go-chi/chi/v5"
	"github.com/twitchtv/twirp"
)

//go:embed template
var htmlFS embed.FS

var ccTempl = template.Must(template.ParseFS(htmlFS, "template/*.html"))

type connectedClient struct {
	Identity   string
	Address    string
	NumTunnels string
}

type connectedInfo struct {
	Node    string
	Clients []connectedClient
}

type clientTunnel struct {
	Hostname string
	Target   string
}

type tunnelsInfo struct {
	Address string
	Tunnels []clientTunnel
}

func TunnelServerHandler(s *Server) http.Handler {
	router := chi.NewRouter()

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		clients := s.TunnelTransport.ListConnected()
		info := connectedInfo{
			Node:    s.Identity().GetAddress(),
			Clients: make([]connectedClient, 0),
		}

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
			info.Clients = append(info.Clients, connectedClient{
				Identity:   fmt.Sprintf("%d/%s", node.GetId(), node.GetAddress()),
				Address:    addr.String(),
				NumTunnels: numTunnels,
			})
		}

		w.Header().Set("content-type", "text/html; charset=utf-8")
		if err := ccTempl.ExecuteTemplate(w, "client_list.html", info); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "error rendering template: %s", err.Error())
		}
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

		info := tunnelsInfo{
			Address: address,
			Tunnels: make([]clientTunnel, 0),
		}

		for _, child := range children {
			hostname := string(child)
			target, ok := tunnelMap[hostname]
			if ok {
				info.Tunnels = append(info.Tunnels, clientTunnel{
					Hostname: hostname,
					Target:   target,
				})
			} else {
				info.Tunnels = append(info.Tunnels, clientTunnel{
					Hostname: hostname,
					Target:   "(unused)",
				})
			}
		}

		w.Header().Set("content-type", "text/html; charset=utf-8")
		if err := ccTempl.ExecuteTemplate(w, "client_tunnels.html", info); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "error rendering template: %s", err.Error())
		}
	})

	return router
}
