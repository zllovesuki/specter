package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util"

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

func (c *Client) startLocalServer(ctx context.Context) {
	if c.ServerListener == nil {
		return
	}

	r := chi.NewRouter()

	r.Post("/unpublish/{hostname}", func(w http.ResponseWriter, r *http.Request) {
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

		c.syncMu.Lock()
		defer c.syncMu.Unlock()

		err = c.UnpublishTunnel(r.Context(), Tunnel{
			Hostname: hostname,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Tunnel %s unpublished from network\n", hostname)
	})

	r.Post("/release/{hostname}", func(w http.ResponseWriter, r *http.Request) {
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

		c.syncMu.Lock()
		defer c.syncMu.Unlock()

		err = c.ReleaseTunnel(r.Context(), Tunnel{
			Hostname: hostname,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Tunnel %s released from network\n", hostname)
	})

	r.Get("/acme/{hostname}", func(w http.ResponseWriter, r *http.Request) {
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

	r.Get("/validate/{hostname}", func(w http.ResponseWriter, r *http.Request) {
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

	r.Get("/ls", func(w http.ResponseWriter, r *http.Request) {
		hostnames, err := c.GetRegisteredHostnames(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.FormatList(hostnames, w)
	})

	// catch-all helper
	routes := make([]string, 0)
	chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		routes = append(routes, fmt.Sprintf("%4s %s", method, route))
		return nil
	})
	routesStr := strings.Join(routes, "\n")
	r.HandleFunc("/*", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s\n", routesStr)
	})

	srv := &http.Server{
		Handler:           r,
		ReadHeaderTimeout: connectTimeout,
		ErrorLog:          util.GetStdLogger(c.Logger, "localServer"),
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	c.Logger.Info("Local server started", zap.String("listen", c.ServerListener.Addr().String()))

	go srv.Serve(c.ServerListener)
}
