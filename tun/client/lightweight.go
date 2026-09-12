package client

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	pkiImpl "go.miragespace.co/specter/pki"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/tun/client/dialer"
	"go.miragespace.co/specter/util"

	"go.uber.org/zap"
)

type LightweightConfig struct {
	Logger    *zap.Logger
	Transport transport.ClientTransport
	PKIClient protocol.PKIService
	Apex      *dialer.ParsedApex
	Target    string
	Token     string
	Output    io.Writer
}

type LightweightClient struct {
	LightweightConfig
	fwd   *forwarder
	route route
	// The first Open response preserves the URL and home across disconnections.
	initialSession *protocol.OpenSessionResponse
}

// attachmentError marks a local failure that another connection cannot fix.
type attachmentError struct {
	error
}

func (e *attachmentError) Unwrap() error { return e.error }

func NewLightweightClient(cfg LightweightConfig) (*LightweightClient, error) {
	target, err := parseTarget(cfg.Target)
	if err != nil {
		return nil, err
	}
	if cfg.Apex == nil || cfg.Transport == nil || cfg.PKIClient == nil {
		return nil, fmt.Errorf("apex, transport, and PKI client are required")
	}
	if cfg.Apex.Host == "" || cfg.Apex.Port < 1 || cfg.Apex.Port > 65535 {
		return nil, fmt.Errorf("invalid apex address")
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.Output == nil {
		cfg.Output = io.Discard
	}
	return &LightweightClient{
		LightweightConfig: cfg,
		fwd:               newForwarder(cfg.Logger),
		route:             route{parsed: target},
	}, nil
}

func (l *LightweightClient) Run(ctx context.Context) error {
	parent := ctx
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	req, err := pkiImpl.CreateRequest(key)
	if err != nil {
		return err
	}
	issueCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	certResp, err := l.PKIClient.RequestCertificate(issueCtx, req)
	cancel()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("obtaining attachment certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(certResp.GetCertDer())
	if err != nil {
		return err
	}
	identity, err := pki.ExtractCertificateIdentity(cert)
	if err != nil {
		return err
	}
	notAfter := cert.NotAfter
	l.Logger.Info("Attachment certificate obtained", zap.Object("identity", identity), zap.Time("notAfter", notAfter))
	if err := l.Transport.WithClientCertificate(tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
	}); err != nil {
		return err
	}
	ctx, cancel = context.WithDeadline(ctx, notAfter)
	defer cancel()
	defer l.fwd.closeAll()
	router := transport.NewStreamRouter(l.Logger, nil, l.Transport)
	router.HandleTunnel(protocol.Stream_DIRECT, func(d *transport.StreamDelegate) {
		link, ok := receiveLink(l.Logger, d)
		if ok {
			l.fwd.handleLink(ctx, link, d, l.route)
		}
	})
	go router.Accept(ctx)
	if l.Token == "" {
		err = l.runEphemeral(ctx)
	} else {
		err = l.runToken(ctx)
	}
	if parent.Err() != nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("attachment certificate expired at %s", notAfter.Format(time.RFC3339))
	}
	return err
}

func (l *LightweightClient) runEphemeral(ctx context.Context) error {
	backoff := time.Second
	for ctx.Err() == nil {
		node := l.initialSession.GetNode()
		if node == nil {
			node = &protocol.Node{Address: l.Apex.String()}
		}
		activated, err := l.attach(ctx, node)
		if ctx.Err() != nil {
			break
		}
		if definite(err) {
			return err
		}
		if unsupported(err) {
			return fmt.Errorf("server %s does not support lightweight tunnels", node.GetAddress())
		}
		if activated {
			backoff = time.Second
		}
		l.Logger.Warn("Retrying tunnel attachment", zap.Error(err), zap.Duration("backoff", backoff))
		timer := time.NewTimer(util.RandomTimeRange(backoff))
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	return ctx.Err()
}

func (l *LightweightClient) dialAttachment(ctx context.Context, node *protocol.Node) (net.Conn, transport.PhysicalConn, error) {
	// Overlay retains this context for the physical connection. Only RPC requests
	// get attempt deadlines; ending a successful unary request must not end traffic.
	conn, err := l.Transport.DialStream(ctx, node, protocol.Stream_RPC)
	if err != nil {
		return nil, nil, err
	}
	provider, ok := conn.(transport.PhysicalConnProvider)
	if !ok || provider.PhysicalConn() == nil {
		conn.Close()
		return nil, nil, &attachmentError{fmt.Errorf("transport does not expose a physical connection")}
	}
	return conn, provider.PhysicalConn(), nil
}

// Each client makes one request on the supplied stream; its caller owns the
// physical connection. Closing the RPC stream must not end other tunnel streams.
func tunnelClientOnStream(conn net.Conn) protocol.TunnelService {
	tp := http.DefaultTransport.(*http.Transport).Clone()
	tp.DisableKeepAlives = true
	tp.DialTLSContext = func(context.Context, string, string) (net.Conn, error) {
		return conn, nil
	}
	return protocol.NewTunnelServiceProtobufClient("https://tunnel", &http.Client{Transport: tp})
}

func (l *LightweightClient) attach(ctx context.Context, node *protocol.Node) (bool, error) {
	conn, pc, err := l.dialAttachment(ctx, node)
	if err != nil {
		return false, err
	}
	defer pc.Close("attachment ended")
	defer conn.Close()
	openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	resp, err := tunnelClientOnStream(conn).OpenEphemeralSession(openCtx, &protocol.OpenEphemeralSessionRequest{})
	cancel()
	if err != nil {
		return false, err
	}
	first := l.initialSession == nil
	if err := l.acceptSession(resp); err != nil {
		return false, &attachmentError{err}
	}
	if !first {
		l.Logger.Info("Tunnel recovered", zap.String("url", l.URL()), zap.String("server", resp.GetNode().GetAddress()))
	}
	select {
	case <-ctx.Done():
	case <-pc.Done():
	}
	l.Logger.Info("Tunnel disconnected", zap.Error(pc.Err()))
	return true, pc.Err()
}

func (l *LightweightClient) acceptSession(resp *protocol.OpenSessionResponse) error {
	if resp.GetHostname() == "" || resp.GetNode().GetAddress() == "" || resp.GetApex() == "" {
		return fmt.Errorf("server returned an empty hostname, identity, or apex")
	}
	if l.initialSession != nil {
		if l.initialSession.GetHostname() != resp.GetHostname() {
			return fmt.Errorf("recovered tunnel hostname changed")
		}
		return nil
	}
	l.initialSession = resp
	l.fwd.rootDomain.Store(resp.GetApex())
	if _, err := fmt.Fprintln(l.Output, l.URL()); err != nil {
		return &attachmentError{fmt.Errorf("printing tunnel URL: %w", err)}
	}
	l.Logger.Info("Tunnel ready", zap.String("url", l.URL()), zap.String("hostname", resp.GetHostname()), zap.String("server", resp.GetNode().GetAddress()))
	return nil
}

func (l *LightweightClient) URL() string {
	hostname := l.initialSession.GetHostname()
	if strings.Contains(hostname, ".") {
		return "https://" + hostname
	}
	authority := hostname + "." + l.initialSession.GetApex()
	if l.Apex.Port != 443 {
		authority = net.JoinHostPort(authority, fmt.Sprint(l.Apex.Port))
	}
	return "https://" + authority
}
