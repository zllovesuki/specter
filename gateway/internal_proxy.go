package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"

	"go.uber.org/zap"
)

const (
	internalProxyNodeAddress = "x-internal-proxy-node-address"
	internalProxyForwarded   = "x-internal-proxy-forwarded"
)

func (g *Gateway) getInternalProxyHandler() func(http.Handler) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   g.RootDomains[0],
	})
	proxy.Transport = &http.Transport{
		DisableKeepAlives:   true,
		MaxIdleConnsPerHost: -1,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			target := rpc.GetNode(ctx)
			if target == nil {
				return nil, transport.ErrNoDirect
			}
			return g.TunnelServer.DialInternal(ctx, target)
		},
	}
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Header.Set(internalProxyForwarded, "true")
		r.Header.Del(internalProxyNodeAddress)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "error proxying internal request: %v", err)
	}
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				forwarded     = r.Header.Get(internalProxyForwarded) != ""
				targetAddress = r.Header.Get(internalProxyNodeAddress)
			)
			if forwarded || targetAddress == "" {
				h.ServeHTTP(w, r)
				return
			}

			target := &protocol.Node{
				Address: targetAddress,
			}
			r = r.WithContext(rpc.WithNode(r.Context(), target))

			g.Logger.Debug("Proxying internal request", zap.Object("target", target))
			proxy.ServeHTTP(w, r)
		})
	}
}
