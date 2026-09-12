package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/go-chi/chi/v5"
)

type connectedClient struct {
	ClientID      string `json:"clientId"`
	Identity      string `json:"identity"`
	Address       string `json:"address"`
	Version       string `json:"version"`
	URL           string `json:"url"`
	SessionMode   string `json:"sessionMode,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	OwnerIdentity string `json:"ownerIdentity,omitempty"`
	OwnerLabel    string `json:"ownerLabel,omitempty"`
	OwnerURL      string `json:"ownerUrl,omitempty"`
}

type connectedInfo struct {
	Node       string            `json:"node"`
	ObservedAt string            `json:"observedAt"`
	Clients    []connectedClient `json:"clients"`
}

type clientTunnel struct {
	Hostname   string `json:"hostname"`
	Target     string `json:"target"`
	Configured string `json:"configured"`
	Registered string `json:"registered"`
}

type tunnelsInfo struct {
	Identity           string         `json:"identity"`
	Address            string         `json:"address"`
	ObservedAt         string         `json:"observedAt"`
	ConfigurationError string         `json:"configurationError,omitempty"`
	RegistrationError  string         `json:"registrationError,omitempty"`
	Tunnels            []clientTunnel `json:"tunnels"`
}

// TunnelServerHandler serves JSON observations for connected clients and their tunnels.
func TunnelServerHandler(s *Server) http.Handler {
	router := chi.NewRouter()

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		clients := s.TunnelTransport.ListConnected()
		info := connectedInfo{
			Node:       s.Identity().GetAddress(),
			ObservedAt: time.Now().UTC().Format(time.RFC3339),
			Clients:    make([]connectedClient, 0, len(clients)),
		}

		clientURLs := make(map[string]string, len(clients))
		for _, h := range clients {
			mode, hostname, owner := s.sessions.describe(h.Physical)
			if hostname != "" && !strings.Contains(hostname, ".") {
				hostname += "." + s.Apex
			}
			clientURL := fmt.Sprintf("/_internal/tun/%d/%s", h.Identity.GetId(), url.PathEscape(h.Identity.GetAddress()))
			clientURLs[h.Identity.GetAddress()] = clientURL
			info.Clients = append(info.Clients, connectedClient{
				ClientID:      strconv.FormatUint(h.Identity.GetId(), 10),
				Identity:      fmt.Sprintf("%d/%s", h.Identity.GetId(), h.Identity.GetAddress()),
				Address:       h.Addr.String(),
				Version:       h.Version,
				URL:           clientURL,
				SessionMode:   mode,
				Hostname:      hostname,
				OwnerIdentity: owner,
				OwnerLabel:    ownerDisplayLabel(owner),
			})
		}
		for i := range info.Clients {
			if owner := info.Clients[i].OwnerIdentity; owner != "" {
				info.Clients[i].OwnerURL = clientURLs[owner]
			}
		}
		sort.Slice(info.Clients, func(i, j int) bool { return info.Clients[i].Identity < info.Clients[j].Identity })

		writeTunnelJSON(w, info)
	})

	router.Get("/{id}/*", func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		address := chi.URLParam(r, "*")
		// Chi matches RawPath when present, so decode that parameter once.
		if r.URL.RawPath != "" {
			address, err = url.PathUnescape(address)
			if err != nil {
				http.Error(w, "invalid client address", http.StatusBadRequest)
				return
			}
		}

		client := &protocol.Node{
			Id:         id,
			Address:    address,
			Rendezvous: true,
		}

		callCtx, cancel := context.WithTimeout(r.Context(), lookupTimeout)
		defer cancel()

		// default to http client pooling
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.DisableKeepAlives = true
		defer t.CloseIdleConnections()
		t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Transport detaches the dial context from the request. This transport
			// is private to the lookup, so explicitly bound its dial to our deadline.
			return s.TunnelTransport.DialStream(callCtx, client, protocol.Stream_RPC)
		}
		c := &http.Client{
			Transport: t,
		}

		rpcClient := protocol.NewClientQueryServiceProtobufClient("http://client", c)

		prefix := tun.ClientHostnamesPrefix(&protocol.ClientToken{
			Token: []byte(client.GetAddress()),
		})
		info := tunnelsInfo{
			Identity:   fmt.Sprintf("%d/%s", id, address),
			Address:    address,
			ObservedAt: time.Now().UTC().Format(time.RFC3339),
			Tunnels:    make([]clientTunnel, 0),
		}

		// These sources answer different questions. Read both within one deadline
		// so an unavailable client or ring cannot hide the other source's data.
		type configurationResult struct {
			response *protocol.ListTunnelsResponse
			err      error
		}
		type registrationResult struct {
			hostnames [][]byte
			err       error
		}
		configurations := make(chan configurationResult, 1)
		registrations := make(chan registrationResult, 1)
		go func(results chan<- configurationResult) {
			response, err := rpcClient.ListTunnels(callCtx, &protocol.ListTunnelsRequest{})
			results <- configurationResult{response: response, err: err}
		}(configurations)
		go func(results chan<- registrationResult) {
			hostnames, err := s.Chord.PrefixList(callCtx, []byte(prefix))
			results <- registrationResult{hostnames: hostnames, err: err}
		}(registrations)

		var configuration configurationResult
		var registration registrationResult
		for configurations != nil || registrations != nil {
			select {
			case configuration = <-configurations:
				configurations = nil
			case registration = <-registrations:
				registrations = nil
			case <-callCtx.Done():
				// Preserve results already completed when cancellation arrived.
				if configurations != nil {
					select {
					case configuration = <-configurations:
					default:
						configuration.err = callCtx.Err()
					}
				}
				if registrations != nil {
					select {
					case registration = <-registrations:
					default:
						registration.err = callCtx.Err()
					}
				}
				configurations, registrations = nil, nil
			}
		}

		configured := make(map[string]string)
		registered := make(map[string]bool)
		if configuration.err != nil {
			info.ConfigurationError = configuration.err.Error()
		} else {
			for _, tunnel := range configuration.response.GetTunnels() {
				configured[tunnel.GetHostname()] = tunnel.GetTarget()
			}
		}
		if registration.err != nil {
			info.RegistrationError = registration.err.Error()
		} else {
			for _, hostname := range registration.hostnames {
				registered[string(hostname)] = true
			}
		}

		hostnames := make(map[string]bool, len(configured)+len(registered))
		for hostname := range configured {
			hostnames[hostname] = true
		}
		for hostname := range registered {
			hostnames[hostname] = true
		}
		for hostname := range hostnames {
			target, hasConfig := configured[hostname]
			info.Tunnels = append(info.Tunnels, clientTunnel{
				Hostname:   hostname,
				Target:     target,
				Configured: observedStatus(hasConfig, configuration.err),
				Registered: observedStatus(registered[hostname], registration.err),
			})
		}
		sort.Slice(info.Tunnels, func(i, j int) bool { return info.Tunnels[i].Hostname < info.Tunnels[j].Hostname })

		writeTunnelJSON(w, info)
	})

	return router
}

func ownerDisplayLabel(identity string) string {
	if remainder, ok := strings.CutPrefix(identity, "v2:"); ok {
		if id, _, ok := strings.Cut(remainder, ":"); ok {
			if value, err := strconv.ParseUint(id, 10, 64); err == nil {
				return strconv.FormatUint(value, 10)
			}
		}
	}
	label := []rune(identity)
	if len(label) <= 24 {
		return identity
	}
	return string(label[:12]) + "…" + string(label[len(label)-8:])
}

func observedStatus(present bool, err error) string {
	if err != nil {
		return "Unknown"
	}
	if present {
		return "Yes"
	}
	return "No"
}

func writeTunnelJSON(w http.ResponseWriter, info any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(info)
}
