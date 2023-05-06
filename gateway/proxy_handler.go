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

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util"

	"go.uber.org/zap"
)

const (
	bufferSize = 1024 * 8
)

var delHeaders = []string{
	"True-Client-IP",
	"X-Real-IP",
	"X-Forwarded-For",
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
	parts := strings.SplitN(host, ".", 2)
	if len(parts) != 2 {
		err = fmt.Errorf("gateway: invalid host for forwarding")
		return
	}
	if util.Contains(g.RootDomains, parts[1]) {
		hostname = parts[0]
	} else {
		hostname = host
	}
	return
}

func (g *Gateway) extractHost(addr string) (host, hostname string, err error) {
	host, _, err = net.SplitHostPort(addr)
	if err != nil {
		return
	}
	hostname, err = g.extractHostname(host)
	return
}

func (g *Gateway) connectDialer(ctx context.Context, r *http.Request) (net.Conn, error) {
	host, hostname, err := g.extractHost(r.Host)
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
	host, hostname, err := g.extractHost(addr)
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
	bufPool := util.NewBufferPool(bufferSize)
	respHandler := func(r *http.Response) error {
		r.Header.Del("alt-svc")
		g.appendHeaders(r.Request.ProtoAtLeast(3, 0))(r.Header)
		return nil
	}

	proxyTransport := &http.Transport{
		MaxConnsPerHost:       100,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       time.Second * 15,
		ResponseHeaderTimeout: time.Second * 5,
		ExpectContinueTimeout: time.Second * 3,
	}
	proxyTransport.DialTLSContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		return g.overlayDialer(ctx, addr)
	}

	proxy := &httputil.ReverseProxy{
		Director:       g.proxyDirector,
		Transport:      proxyTransport,
		BufferPool:     bufPool,
		ErrorHandler:   g.errorHandler,
		ModifyResponse: respHandler,
		ErrorLog:       proxyLogger,
	}

	return proxy
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
