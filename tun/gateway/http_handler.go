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
)

const (
	bufferSize = 1024 * 16
)

var delHeaders = []string{
	"True-Client-IP",
	"X-Real-IP",
	"X-Forwarded-For",
}

func (g *Gateway) httpHandler() http.Handler {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = req.TLS.ServerName
			for _, header := range delHeaders {
				req.Header.Del(header)
			}
		},
		Transport: &http.Transport{
			DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
				parts := strings.SplitN(addr, ".", 2)
				g.Logger.Debug("dialing http connection", zap.String("hostname", parts[0]))
				return g.Tun.Dial(c, &protocol.Link{
					Alpn:     protocol.Link_HTTP,
					Hostname: parts[0],
				})
			},
			MaxConnsPerHost:       15,
			MaxIdleConnsPerHost:   3,
			IdleConnTimeout:       time.Minute,
			ResponseHeaderTimeout: time.Second * 30,
			ExpectContinueTimeout: time.Second * 3,
		},
		BufferPool:   NewBufferPool(bufferSize),
		ErrorHandler: g.errorHandler,
		ModifyResponse: func(r *http.Response) error {
			r.Header.Del("alt-svc")
			return nil
		},
		ErrorLog: func() *log.Logger {
			l, _ := zap.NewStdLogAt(g.Logger, zapcore.ErrorLevel)
			return l
		}(),
	}
}

func (g *Gateway) errorHandler(rw http.ResponseWriter, r *http.Request, e error) {
	if errors.Is(e, tun.ErrDestinationNotFound) {
		rw.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(rw, "Destination %s not found on the Chord network.", r.URL.Hostname())
		return
	}

	g.Logger.Debug("forwarding http/https request", zap.Error(e))

	if tun.IsTimeout(e) {
		rw.WriteHeader(http.StatusGatewayTimeout)
		fmt.Fprintf(rw, "Destination %s is taking too long to respond.", r.URL.Hostname())
		return
	}
	if errors.Is(e, context.Canceled) {
		// this is expected
		return
	}

	g.Logger.Error("forwarding to client", zap.Error(e))
	rw.WriteHeader(http.StatusServiceUnavailable)
	fmt.Fprint(rw, "An unexpected error has occurred while attempting to forward to destination.")
}
