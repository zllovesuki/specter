package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/ui"
	"go.miragespace.co/specter/util"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

var _ protocol.ClientQueryService = (*Client)(nil)

func (c *Client) attachRPC(ctx context.Context, router *transport.StreamRouter) {
	queryTwirp := protocol.NewClientQueryServiceServer(c)

	rpcHandler := chi.NewRouter()
	rpcHandler.Use(middleware.Recoverer)
	rpcHandler.Use(util.LimitBody(1 << 10)) // 1KB
	rpcHandler.Mount(queryTwirp.PathPrefix(), queryTwirp)

	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return rpc.WithDelegation(ctx, c.(*transport.StreamDelegate))
		},
		MaxHeaderBytes:    1 << 10, // 1KB
		ReadHeaderTimeout: time.Second * 3,
		Handler:           rpcHandler,
		ErrorLog:          util.GetStdLogger(c.Logger, "queryServer"),
	}

	go srv.Serve(c.rpcAcceptor)

	router.HandleTunnel(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		c.rpcAcceptor.Handle(delegate)
	})
}

func (c *Client) ListTunnels(ctx context.Context, _ *protocol.ListTunnelsRequest) (*protocol.ListTunnelsResponse, error) {
	c.configMu.RLock()
	cfg := c.Configuration.clone()
	c.configMu.RUnlock()

	tunnels := make([]*protocol.ClientTunnel, 0)
	for _, tunnel := range cfg.Tunnels {
		tunnels = append(tunnels, &protocol.ClientTunnel{
			Hostname: tunnel.Hostname,
			Target:   tunnel.Target,
		})
	}

	return &protocol.ListTunnelsResponse{
		Tunnels: tunnels,
	}, nil
}

type ClientStatus struct {
	Apex            string           `json:"apex"`
	ConnectedNodes  []*protocol.Node `json:"connectedNodes"`
	Synchronization SyncResult       `json:"synchronization"`
	Pending         bool             `json:"pending"`
	RetryAt         *time.Time       `json:"retryAt,omitempty"`
}

func (c *Client) getStatus() ClientStatus {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	c.syncStateMu.RLock()
	defer c.syncStateMu.RUnlock()

	result := c.lastSync
	result.Tunnels = make([]TunnelSyncResult, 0, len(c.Configuration.Tunnels))
	for _, tunnel := range c.Configuration.Tunnels {
		outcome := TunnelSyncResult{Hostname: tunnel.Hostname, Target: tunnel.Target}
		for _, previous := range c.lastSync.Tunnels {
			if previous.Hostname == tunnel.Hostname && previous.Target == tunnel.Target {
				outcome = previous
				break
			}
		}
		result.Tunnels = append(result.Tunnels, outcome)
	}
	status := ClientStatus{
		Apex: c.Configuration.Apex, ConnectedNodes: c.getConnectedNodes(),
		Synchronization: result, Pending: result.pendingPublication(),
	}
	if status.ConnectedNodes == nil {
		status.ConnectedNodes = []*protocol.Node{}
	}
	if status.Pending {
		next := c.nextSync
		status.RetryAt = &next
	}
	return status
}

func writeJSONResult(w http.ResponseWriter, status int, result any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(result)
}

func writeActionError(w http.ResponseWriter, err error) {
	var saveError *ConfigSaveError
	writeJSONResult(w, http.StatusInternalServerError, SyncResult{
		Applied: errors.As(err, &saveError), Error: err.Error(), Tunnels: []TunnelSyncResult{},
	})
}

func (c *Client) localHandler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Heartbeat("/healthz"))

	api := chi.NewRouter()

	api.Post("/reload", func(w http.ResponseWriter, r *http.Request) {
		c.Logger.Info("Received request from API, reloading config")
		result := c.doReload(r.Context())
		if result.Error == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		status := http.StatusInternalServerError
		if !result.Applied {
			status = http.StatusBadRequest
		}
		writeJSONResult(w, status, result)
	})

	api.Get("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSONResult(w, http.StatusOK, c.getStatus())
	})

	api.Get("/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := c.GetCurrentConfig()
		f, err := os.Open(cfg.path)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer f.Close()
		io.Copy(w, f)
	})

	api.Post("/unpublish/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		hostname := chi.URLParam(r, "hostname")
		hostname, err := url.PathUnescape(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hostname, err = acme.Normalize(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = c.UnpublishTunnel(r.Context(), Tunnel{
			Hostname: hostname,
		})
		if err != nil {
			writeActionError(w, err)
			return
		}

		fmt.Fprintf(w, "Tunnel %s unpublished from network\n", hostname)
	})

	api.Post("/release/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		hostname := chi.URLParam(r, "hostname")
		hostname, err := url.PathUnescape(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hostname, err = acme.Normalize(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = c.ReleaseTunnel(r.Context(), Tunnel{
			Hostname: hostname,
		})
		if err != nil {
			writeActionError(w, err)
			return
		}

		fmt.Fprintf(w, "Tunnel %s released from network\n", hostname)
	})

	api.Get("/acme/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		hostname := chi.URLParam(r, "hostname")
		hostname, err := url.PathUnescape(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hostname, err = acme.Normalize(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := c.GetAcmeInstruction(r.Context(), hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.FormatAcme(resp, w)
	})

	api.Get("/validate/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		hostname := chi.URLParam(r, "hostname")
		hostname, err := url.PathUnescape(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hostname, err = acme.Normalize(hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := c.RequestAcmeValidation(r.Context(), hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.FormatValidate(hostname, resp, w)
	})

	api.Get("/ls", func(w http.ResponseWriter, r *http.Request) {
		hostnames, err := c.GetRegisteredHostnames(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.FormatList(hostnames, w)
	})

	r.Mount("/api", api)
	r.Handle("/ui/*", http.StripPrefix("/ui", ui.Assets()))
	r.Handle("/", ui.ClientPage())

	return r
}

func (c *Client) startLocalServer(ctx context.Context) {
	if c.ServerListener == nil {
		return
	}

	srv := &http.Server{
		Handler:           c.localHandler(),
		ReadHeaderTimeout: connectTimeout,
		ErrorLog:          util.GetStdLogger(c.Logger, "localServer"),
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	c.Logger.Info("Local server started", zap.String("listen", c.ServerListener.Addr().String()))

	go srv.Serve(c.ServerListener)
}
