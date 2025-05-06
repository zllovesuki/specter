package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util"

	"github.com/alecthomas/units"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

var delHeaders = []string{
	"True-Client-IP",
	"X-Real-IP",
	"X-Forwarded-For",
}

const (
	proxyHeaderTimeout = time.Second * 300 // global limit of how long the gateway will wait for response
)

// inspiration from https://blog.cloudflare.com/eliminating-cold-starts-with-cloudflare-workers/
// warm the route cache when tls handshake begins
func (g *Gateway) HandshakeEarlyHint(sni string) {
	if g.HandshakeHintFunc == nil {
		return
	}
	hostname, err := g.extractHostname(sni)
	if err != nil {
		return
	}
	if util.Contains(g.RootDomains, hostname) {
		return
	}
	g.Logger.Debug("Handshake early hint", zap.String("hostname", hostname))
	// TODO: implement a limiter
	go g.HandshakeHintFunc(hostname)
}

func (g *Gateway) httpConnect(w http.ResponseWriter, r *http.Request) {
	remote, err := g.connectDialer(r.Context(), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	status := protocol.TunnelStatus{}
	err = rpc.Receive(remote, &status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		remote.Close()
		return
	}
	if status.Status != protocol.TunnelStatusCode_STATUS_OK {
		http.Error(w, status.GetError(), http.StatusServiceUnavailable)
		remote.Close()
		return
	}

	rc := http.NewResponseController(w)
	local, rw, err := rc.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		remote.Close()
		return
	}

	connectResp := http.Response{
		StatusCode: http.StatusOK,
		Proto:      r.Proto,
		ProtoMajor: r.ProtoMajor,
		ProtoMinor: r.ProtoMinor,
	}
	connectResp.Write(rw)
	rw.Flush()

	tun.Pipe(local, remote)
}

func (g *Gateway) extractHostname(host string) (hostname string, err error) {
	if net.ParseIP(host) != nil {
		err = fmt.Errorf("gateway: hostname cannot be IP")
		return
	}
	if strings.Count(host, ".") < 2 {
		err = fmt.Errorf("gateway: too few labels in hostname")
		return
	}
	parts := strings.SplitN(host, ".", 2)
	if len(parts) != 2 {
		err = fmt.Errorf("gateway: invalid hostname for forwarding")
		return
	}
	if util.Contains(g.RootDomains, parts[1]) {
		hostname = parts[0]
	} else {
		hostname = host
	}
	hostname = strings.ToLower(hostname)
	return
}

func (g *Gateway) parseAddr(addr string) (host, hostname string, err error) {
	host, _, err = net.SplitHostPort(addr)
	if err != nil {
		return
	}
	hostname, err = g.extractHostname(host)
	return
}

func (g *Gateway) connectDialer(ctx context.Context, r *http.Request) (net.Conn, error) {
	host, hostname, err := g.parseAddr(r.Host)
	if err != nil {
		return nil, err
	}
	connectHostname.Add(hostname, 1)
	g.Logger.Debug("Dialing to client via overlay (HTTP Connect)", zap.String("hostname", hostname), zap.String("req.URL.Host", host))
	return g.TunnelServer.DialClient(ctx, &protocol.Link{
		Alpn:     protocol.Link_TCP,
		Hostname: hostname,
		Remote:   r.RemoteAddr,
	})
}

func (g *Gateway) overlayDialer(ctx context.Context, addr string) (net.Conn, error) {
	host, hostname, err := g.parseAddr(addr)
	if err != nil {
		return nil, err
	}
	g.Logger.Debug("Dialing to client via overlay (HTTP)", zap.String("hostname", hostname), zap.String("req.URL.Host", host))
	return g.TunnelServer.DialClient(ctx, &protocol.Link{
		Alpn:     protocol.Link_HTTP,
		Hostname: hostname,
		Remote:   g.TunnelServer.Identity().GetAddress(), // include gateway address as placeholder
	})
}

func (g *Gateway) forwardTCP(ctx context.Context, host string, remote string, conn DeadlineReadWriteCloser) error {
	var (
		hostname string
		c        net.Conn
		err      error
	)
	defer func() {
		if err != nil {
			tun.SendStatusProto(conn, err)
			conn.Close()
			return
		}
		tun.Pipe(conn, c)
	}()

	// because of quic's early connection, the client need to "poke" us before
	// we can actually accept a stream, despite .OpenStreamSync
	conn.SetReadDeadline(time.Now().Add(time.Second * 3))
	err = tun.DrainStatusProto(conn)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})

	hostname, err = g.extractHostname(host)
	if err != nil {
		return err
	}

	g.Logger.Debug("Dialing to client via overlay (TCP)", zap.String("hostname", host))
	c, err = g.TunnelServer.DialClient(ctx, &protocol.Link{
		Alpn:     protocol.Link_TCP,
		Hostname: hostname,
		Remote:   remote,
	})
	if err != nil {
		return err
	}

	return nil
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
	req.URL.Host = req.URL.Hostname()
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

func (g *Gateway) proxyHandler(proxyLogger *log.Logger) http.Handler {
	respHandler := func(r *http.Response) error {
		r.Header.Del("alt-svc")
		g.appendHeaders(r.Request.ProtoAtLeast(3, 0))(r.Header)
		return nil
	}

	g.Logger.Info("Using buffer sizes for proxy",
		zap.String("transport", units.Base2Bytes(g.Options.TransportBufferSize).String()),
		zap.String("proxy", units.Base2Bytes(g.Options.ProxyBufferSize).String()),
	)

	proxyTransport := &http.Transport{
		MaxConnsPerHost:       30,
		MaxIdleConnsPerHost:   5,
		DisableCompression:    true,
		IdleConnTimeout:       time.Second * 30,
		ResponseHeaderTimeout: proxyHeaderTimeout,
		ExpectContinueTimeout: time.Second * 5,
		WriteBufferSize:       g.Options.TransportBufferSize,
		ReadBufferSize:        g.Options.TransportBufferSize,
	}
	proxyTransport.DialTLSContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		return g.overlayDialer(ctx, addr)
	}

	bufPool := util.NewBufferPool(g.Options.ProxyBufferSize)
	proxy := &httputil.ReverseProxy{
		Director:       g.proxyDirector,
		Transport:      proxyTransport,
		BufferPool:     bufPool,
		ErrorHandler:   g.errorHandler,
		ModifyResponse: respHandler,
		ErrorLog:       proxyLogger,
		FlushInterval:  -1,
	}

	router := chi.NewRouter()

	g.mountCgiHandler(router)
	router.Handle("/*", proxy)

	return router
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

	logger := g.Logger.With(zap.String("hostname", r.URL.Hostname()))

	logger.Debug("error forwarding http request", zap.Error(e))

	if tun.IsTimeout(e) {
		w.WriteHeader(http.StatusGatewayTimeout)
		fmt.Fprintf(w, "Destination %s is taking too long to respond.", r.URL.Hostname())
		return
	}

	logger.Error("error forwarding to client", zap.Error(e))
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprint(w, "An unexpected error has occurred while attempting to forward to destination.")
}
