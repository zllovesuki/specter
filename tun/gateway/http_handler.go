package gateway

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"moul.io/zapfilter"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/http2"
)

const (
	bufferSize = 1024 * 4
)

var delHeaders = []string{
	"True-Client-IP",
	"X-Real-IP",
	"X-Forwarded-For",
}

func (g *Gateway) overlayDialer(ctx context.Context, _, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var hostname string
	parts := strings.SplitN(host, ".", 2)
	if strings.HasSuffix(parts[1], g.RootDomain) {
		hostname = parts[0]
	} else {
		// TODO: custom hostname support
		return nil, fmt.Errorf("not implemented")
	}
	g.Logger.Debug("Dialing to client via overlay", zap.String("hostname", hostname), zap.String("req.URL.Host", host))
	return g.Tun.Dial(ctx, &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: hostname,
	})
}

func (g *Gateway) proxyDirector(req *http.Request) {
	req.URL.Scheme = "https"
	// most browsers' connection coalescing behavior for http3 is the same as http2,
	// as they could be reusing the same tcp/quic connection for different hosts.
	// https://daniel.haxx.se/blog/2016/08/18/http2-connection-coalescing/
	// https://mailarchive.ietf.org/arch/msg/quic/ffjARd8-IobIE2T9_r5u9hBDbuk/
	if req.ProtoAtLeast(2, 0) {
		req.URL.Host = req.Host
	} else {
		req.URL.Host = req.TLS.ServerName
	}
	for _, header := range delHeaders {
		req.Header.Del(header)
	}
	if g.GatewayPort == 443 {
		req.Header.Set("X-Forwarded-Host", req.URL.Host)
	} else {
		req.Header.Set("X-Forwarded-Host", fmt.Sprintf("%s:%d", req.URL.Host, g.GatewayPort))
	}
	req.Header.Set("X-Forwarded-Proto", "https")
}

func (g *Gateway) httpHandler() (http.Handler, http.Handler) {
	bufPool := NewBufferPool(bufferSize)
	respHandler := func(r *http.Response) error {
		r.Header.Del("alt-svc")
		g.appendHeaders(r.Request.ProtoAtLeast(3, 0))(r.Header)
		return nil
	}

	// filter out unproductive messages
	filteredLogger := zap.New(zapfilter.NewFilteringCore(
		g.Logger.Core(),
		func(e zapcore.Entry, f []zapcore.Field) bool {
			return !strings.HasPrefix(e.Message, "http: URL query contains semicolon")
		}),
	)
	proxyLogger, err := zap.NewStdLogAt(filteredLogger, zapcore.ErrorLevel)
	if err != nil {
		g.Logger.Fatal("error getting proxy logger", zap.Error(err))
	}

	// configure h1 transport and h2 transport separately, while letting the h2 one
	// uses the settings from h1 transport
	h1Transport := &http.Transport{
		DialTLSContext:        g.overlayDialer,
		MaxConnsPerHost:       30,
		MaxIdleConnsPerHost:   3,
		IdleConnTimeout:       time.Minute,
		ResponseHeaderTimeout: time.Second * 30,
		ExpectContinueTimeout: time.Second * 3,
	}
	h2Transport, _ := http2.ConfigureTransports(h1Transport)
	h2Transport.DialTLSContext = func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
		return g.overlayDialer(ctx, network, addr)
	}
	h2Transport.ConnPool = nil
	h1Transport.TLSNextProto = nil

	h1Proxy := &httputil.ReverseProxy{
		Director:       g.proxyDirector,
		Transport:      h1Transport,
		BufferPool:     bufPool,
		ErrorHandler:   g.errorHandler,
		ModifyResponse: respHandler,
		ErrorLog:       proxyLogger,
	}
	h2Proxy := &httputil.ReverseProxy{
		Director:       g.proxyDirector,
		Transport:      h2Transport,
		BufferPool:     bufPool,
		ErrorHandler:   g.errorHandler,
		ModifyResponse: respHandler,
		ErrorLog:       proxyLogger,
	}
	return h1Proxy, h2Proxy
}

func (g *Gateway) errorHandler(w http.ResponseWriter, r *http.Request, e error) {
	g.appendHeaders(r.ProtoAtLeast(3, 0))(w.Header())

	if errors.Is(e, tun.ErrDestinationNotFound) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Destination %s not found on the specter network.", r.URL.Hostname())
		return
	}

	if errors.Is(e, tun.ErrTunnelClientNotConnected) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "Destination %s is not connected to specter network.", r.URL.Hostname())
		return
	}

	if errors.Is(e, context.Canceled) ||
		errors.Is(e, io.EOF) {
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
