package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"

	"go.uber.org/zap"
)

const (
	internalProxyNodeAddress = "x-internal-proxy-node-address"
	internalProxyNodeId      = "x-internal-proxy-node-id"
	internalProxyForwarded   = "x-internal-proxy-forwarded"
)

func (g *Gateway) getInternalProxyHandler() func(http.Handler) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   g.RootDomain,
	})
	proxy.Transport = &http.Transport{
		DisableKeepAlives:   false,
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
		r.Header.Del(internalProxyNodeId)
	}
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				forwarded     = r.Header.Get(internalProxyForwarded) != ""
				targetAddress = r.Header.Get(internalProxyNodeAddress)
				targetIdStr   = r.Header.Get(internalProxyNodeId)
			)
			if forwarded || targetAddress == "" || targetIdStr == "" {
				h.ServeHTTP(w, r)
				return
			}

			targerId, err := strconv.ParseUint(targetIdStr, 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "error parsing %s: %v", internalProxyNodeId, err)
				return
			}

			target := &protocol.Node{
				Address: targetAddress,
				Id:      targerId,
			}
			r = r.WithContext(rpc.WithNode(r.Context(), target))

			g.Logger.Debug("Proxying internal request", zap.Object("target", target))
			proxy.ServeHTTP(w, r)
		})
	}
}
