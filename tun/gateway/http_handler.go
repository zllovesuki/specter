package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/http2"
)

const (
	bufferSize        = 1024 * 16
	invalidServerName = "gotofail.xxx"
)

var delHeaders = []string{
	"True-Client-IP",
	"X-Real-IP",
	"X-Forwarded-For",
}

func (g *Gateway) httpHandler() http.Handler {
	transport := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			parts := strings.SplitN(addr, ".", 2)
			g.Logger.Debug("Dialing to client via overlay", zap.String("hostname", parts[0]), zap.String("tls.ServerName", addr))
			return g.Tun.Dial(ctx, &protocol.Link{
				Alpn:     protocol.Link_HTTP,
				Hostname: parts[0],
			})
		},
		MaxConnsPerHost:       30,
		MaxIdleConnsPerHost:   3,
		IdleConnTimeout:       time.Minute,
		ResponseHeaderTimeout: time.Second * 30,
		ExpectContinueTimeout: time.Second * 3,
	}
	if err := http2.ConfigureTransport(transport); err != nil {
		g.Logger.Fatal("error configuring transport", zap.Error(err))
	}
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "https"
			req.URL.Host = req.TLS.ServerName
			for _, header := range delHeaders {
				req.Header.Del(header)
			}
		},
		Transport:    transport,
		BufferPool:   NewBufferPool(bufferSize),
		ErrorHandler: g.errorHandler,
		ModifyResponse: func(r *http.Response) error {
			r.Header.Del("alt-svc")
			g.appendHeaders(r.Request.ProtoAtLeast(3, 0))(r.Header)
			return nil
		},
		ErrorLog: func() *log.Logger {
			l, _ := zap.NewStdLogAt(g.Logger, zapcore.ErrorLevel)
			return l
		}(),
	}
}

func (g *Gateway) errorHandler(w http.ResponseWriter, r *http.Request, e error) {
	g.appendHeaders(r.ProtoAtLeast(3, 0))(w.Header())

	if errors.Is(e, tun.ErrDestinationNotFound) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Destination %s not found on the specter network.", r.TLS.ServerName)
		return
	}

	if errors.Is(e, tun.ErrTunnelClientNotConnected) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "Destination %s is not connected to specter network.", r.TLS.ServerName)
		return
	}

	if errors.Is(e, context.Canceled) {
		// this is expected
		return
	}

	g.Logger.Debug("error forwarding http/https request", zap.Error(e))

	if tun.IsTimeout(e) {
		w.WriteHeader(http.StatusGatewayTimeout)
		fmt.Fprintf(w, "Destination %s is taking too long to respond.", r.URL.Hostname())
		return
	}

	g.Logger.Error("forwarding to client", zap.Error(e))
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprint(w, "An unexpected error has occurred while attempting to forward to destination.")
}
