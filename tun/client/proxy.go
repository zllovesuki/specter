package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util"
	"go.miragespace.co/specter/util/acceptor"
	"go.miragespace.co/specter/util/pipe"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type httpProxy struct {
	acceptor  *acceptor.HTTP2Acceptor
	forwarder *http.Server
}

type httpReqCtxKey string

const (
	ctxStartTime = httpReqCtxKey("start-time")
)

func injectStartTime(proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := context.WithValue(r.Context(), ctxStartTime, start)
		r = r.WithContext(ctx)
		proxy.ServeHTTP(w, r)
	})
}

func (c *Client) forwardStream(ctx context.Context, hostname string, remote net.Conn, r route) {
	var (
		u      *url.URL = r.parsed
		target string
		local  net.Conn
		err    error
	)
	logger := c.Logger.With(zap.String("hostname", hostname), zap.String("target", u.String()))
	switch u.Scheme {
	case "tcp":
		dialer := &net.Dialer{
			Timeout: time.Second * 3,
		}
		target = u.Host
		local, err = dialer.DialContext(ctx, "tcp", u.Host)
	case "unix", "winio":
		target = u.Path
		local, err = pipe.DialPipe(ctx, u.Path)
	default:
		err = fmt.Errorf("unknown scheme: %s", u.Scheme)
	}
	if err != nil {
		logger.Error("Error dialing to target", zap.String("target", target), zap.Error(err))
		tun.SendStatusProto(remote, err)
		remote.Close()
		return
	}
	tun.SendStatusProto(remote, nil)
	tun.Pipe(remote, local)
}

func (c *Client) getHTTPProxy(ctx context.Context, hostname string, r route) *httpProxy {
	proxy, loaded := c.proxies.LoadOrStoreLazy(hostname, func() *httpProxy {
		var (
			u      *url.URL = r.parsed
			isPipe          = false
		)

		logger := c.Logger.With(zap.String("hostname", hostname), zap.String("target", u.String()))
		logger.Info("Creating new proxy")

		tp := &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         u.Host,
				InsecureSkipVerify: r.insecure,
			},
			MaxIdleConns:          10,
			IdleConnTimeout:       time.Second * 30,
			ResponseHeaderTimeout: connectTimeout,
			ForceAttemptHTTP2:     true,
		}
		switch u.Scheme {
		case "unix", "winio":
			isPipe = true
			tp.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return pipe.DialPipe(ctx, u.Path)
			}
		}

		proxy := httputil.NewSingleHostReverseProxy(u)
		d := proxy.Director
		// https://stackoverflow.com/a/53007606
		// need to overwrite Host field
		proxy.Director = func(r *http.Request) {
			d(r)
			if isPipe {
				r.Host = "pipe"
				r.URL.Host = "pipe"
				r.URL.Scheme = "http"
				r.URL.Path = strings.ReplaceAll(r.URL.Path, u.Path, "")
			} else {
				r.Host = u.Host
			}
		}
		proxy.Transport = tp
		proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, e error) {
			if errors.Is(e, context.Canceled) ||
				errors.Is(e, io.EOF) {
				// this is expected
				return
			}
			logger.Error("Error forwarding http/https request", zap.Object("request", (*encRequest)(r)), zap.Error(e))
			rw.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(rw, "Forwarding target returned error: %s", e.Error())
		}
		proxy.ModifyResponse = func(r *http.Response) error {
			c.Logger.Debug("Access Log", zap.Object("request", (*encRequest)(r.Request)), zap.Object("response", (*encResponse)(r)))
			return nil
		}
		proxy.ErrorLog = util.GetStdLogger(logger, "targetProxy")
		proxy.BufferPool = util.NewBufferPool(1024 * 16)

		return &httpProxy{
			acceptor: acceptor.NewH2Acceptor(nil),
			forwarder: &http.Server{
				Handler:           injectStartTime(proxy),
				ErrorLog:          zap.NewStdLog(c.Logger),
				ReadHeaderTimeout: time.Second * 15,
			},
		}
	})
	if !loaded {
		go proxy.forwarder.Serve(proxy.acceptor)
	}
	return proxy
}

type encRequest http.Request

var _ zapcore.ObjectMarshaler = (*encRequest)(nil)

func (r *encRequest) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("method", r.Method)
	enc.AddString("path", r.RequestURI)
	proxied := r.Header.Get("x-forwarded-for")
	if len(proxied) > 0 {
		ips := strings.Split(proxied, ",")
		for i, ip := range ips {
			ips[i] = strings.TrimSpace(ip)
		}
		enc.AddString("client", ips[0])
		enc.AddString("via", ips[1])
	}
	return nil
}

type encResponse http.Response

var _ zapcore.ObjectMarshaler = (*encResponse)(nil)

func (r *encResponse) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	ctx := r.Request.Context()
	start := ctx.Value(ctxStartTime).(time.Time)
	enc.AddInt("code", r.StatusCode)
	enc.AddInt64("bytes", r.ContentLength)
	enc.AddDuration("duration", time.Since(start))
	return nil
}
