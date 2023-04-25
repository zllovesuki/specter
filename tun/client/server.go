package client

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/util"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func (c *Client) startServer(ctx context.Context) {
	if c.ServerListener == nil {
		return
	}

	r := chi.NewRouter()
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
