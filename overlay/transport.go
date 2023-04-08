package overlay

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/atomic"
	"kon.nect.sh/specter/util/reuse"

	"github.com/avast/retry-go/v4"
	"github.com/quic-go/quic-go"
	"github.com/zhangyunhao116/skipmap"
	uberAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

var (
	_ transport.Transport       = (*QUIC)(nil)
	_ transport.ClientTransport = (*QUIC)(nil)
)

func NewQUIC(conf TransportConfig) *QUIC {
	if conf.VirtualTransport && conf.UseCertificateIdentity {
		panic("cannot enable UseCertificateIdentity and VirtualTransport in the same transport")
	}
	return &QUIC{
		TransportConfig: conf,

		cachedConnections: skipmap.NewString[*nodeConnection](),
		cachedMutex:       atomic.NewKeyedRWMutex(),

		streamChan: make(chan *transport.StreamDelegate, 32),
		dgramChan:  make(chan *transport.DatagramDelegate, 32),

		rttChan: make(chan *transport.DatagramDelegate, 8),
		rttMap:  skipmap.NewString[*skipmap.Uint64Map[int64]](),

		started: uberAtomic.NewBool(false),
		closed:  uberAtomic.NewBool(false),
	}
}

func (t *QUIC) makeCachedKey(peer *protocol.Node) string {
	qMapKey := peer.GetAddress() + "/"
	if peer.GetUnknown() {
		qMapKey = qMapKey + "-1"
	} else {
		if t.VirtualTransport {
			qMapKey = qMapKey + "PHY"
		} else {
			qMapKey = qMapKey + strconv.FormatUint(peer.GetId(), 10)
		}
	}
	return qMapKey
}

func (t *QUIC) getCachedConnection(ctx context.Context, peer *protocol.Node) (quic.EarlyConnection, error) {
	var (
		qKey = t.makeCachedKey(peer)
		q    quic.EarlyConnection
	)

	if t.Endpoint.GetAddress() == peer.GetAddress() {
		return nil, fmt.Errorf("creating a new QUIC connection to the ourselves is not allowed")
	}

	if err := retry.Do(func() error {
		rUnlock := t.cachedMutex.RLock(qKey)
		if cached, ok := t.cachedConnections.Load(qKey); ok {
			rUnlock()
			q = cached.quic
			return nil
		}
		rUnlock()

		if peer.GetRendezvous() || peer.GetAddress() == "" {
			return transport.ErrNoDirect
		}

		t.Logger.Debug("Creating new QUIC connection", zap.Object("peer", peer))

		dialCtx, dialCancel := context.WithTimeout(ctx, transport.ConnectTimeout)
		defer dialCancel()

		listenCfg := &net.ListenConfig{
			Control: reuse.Control,
		}

		pconn, err := listenCfg.ListenPacket(dialCtx, "udp", ":0")
		if err != nil {
			return err
		}

		addr, err := net.ResolveUDPAddr("udp", peer.GetAddress())
		if err != nil {
			return err
		}

		cfg := t.ClientTLS.Clone()
		if cert, ok := t.clientCert.Load().(tls.Certificate); ok {
			cfg.Certificates = []tls.Certificate{cert}
		}

		newQ, err := quic.DialEarlyContext(dialCtx, pconn, addr, peer.GetAddress(), cfg, quicConfig)
		if err != nil {
			return err
		}

		reused, err := t.handleOutgoing(ctx, newQ)
		if err != nil {
			return err
		}

		q = reused
		return nil
	},
		retry.Attempts(2),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			t.Logger.Info("Potential connection reuse conflict, retrying to get previously cached connection", zap.Object("peer", peer), zap.Error(err))
		}),
		retry.RetryIf(func(err error) bool {
			return strings.Contains(err.Error(), reuseErrorState)
		}),
	); err != nil {
		if err != transport.ErrNoDirect {
			t.Logger.Error("Failed to establish connection", zap.Object("peer", peer), zap.Error(err))
		}
		return nil, err
	}

	t.background(ctx)

	return q, nil
}

func (t *QUIC) WithClientCertificate(cert tls.Certificate) error {
	if len(cert.Certificate) == 0 {
		return transport.ErrNoCertificate
	}
	if cert.PrivateKey == nil {
		return transport.ErrNoCertificate
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return err
	}
	identity, err := pki.ExtractCertificateIdentity(parsed)
	if err != nil {
		return err
	}

	t.Logger.Debug("Using client certificate", zap.Object("identity", identity))
	t.Endpoint.Id = identity.ID
	t.clientCert.Store(cert)

	return nil
}

func (t *QUIC) Identity() *protocol.Node {
	return t.Endpoint
}

func (t *QUIC) DialStream(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	if t.closed.Load() {
		return nil, transport.ErrClosed
	}

	if peer.GetAddress() == t.Endpoint.GetAddress() && t.VirtualTransport {
		c1, c2 := net.Pipe()
		t.streamChan <- &transport.StreamDelegate{
			Identity: &protocol.Node{
				Address: t.Endpoint.GetAddress(),
				Id:      peer.GetId(),
			},
			Conn: c2,
			Kind: kind,
		}
		return c1, nil
	}

	q, err := t.getCachedConnection(ctx, peer)
	if err != nil {
		return nil, fmt.Errorf("creating quic connection: %w", err)
	}

	stream, err := q.OpenStream()
	if err != nil {
		return nil, err
	}

	var rr protocol.Stream
	if t.VirtualTransport {
		rr = protocol.Stream{
			Type: kind,
			Target: &protocol.Node{
				Id: peer.GetId(),
			},
		}
	} else {
		rr = protocol.Stream{
			Type: kind,
		}
	}
	stream.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.Send(stream, &rr)
	if err != nil {
		return nil, err
	}
	stream.SetDeadline(time.Time{})

	// t.Logger.Debug("Created new Stream",
	// 	zap.String("kind", kind.String()),
	// 	zap.String("remote", q.RemoteAddr().String()),
	// 	zap.String("local", q.LocalAddr().String()))

	return WrapQuicConnection(stream, q), nil
}

func (t *QUIC) AcceptStream() <-chan *transport.StreamDelegate {
	return t.streamChan
}

func (t *QUIC) SupportDatagram() bool {
	return quicConfig.EnableDatagrams
}

func (t *QUIC) ReceiveDatagram() <-chan *transport.DatagramDelegate {
	return t.dgramChan
}

func (t *QUIC) SendDatagram(peer *protocol.Node, buf []byte) error {
	qKey := t.makeCachedKey(peer)
	if r, ok := t.cachedConnections.Load(qKey); ok {
		data := &protocol.Datagram{
			Type: protocol.Datagram_DATA,
			Data: buf,
		}
		b, err := data.MarshalVT()
		if err != nil {
			return err
		}
		return r.quic.SendMessage(b)
	}
	return transport.ErrNoDirect
}

func (t *QUIC) handleIncoming(ctx context.Context, q quic.EarlyConnection) (quic.EarlyConnection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, quicConfig.HandshakeIdleTimeout)
	defer openCancel()

	stream, err := q.OpenStreamSync(openCtx)
	if err != nil {
		return nil, err
	}
	defer WrapQuicConnection(stream, q).Close()

	c, reused, err := t.reuseConnection(ctx, q, stream, directionIncoming)
	if err != nil {
		return nil, err
	}

	if !reused {
		t.handlePeer(ctx, c.quic, c.peer, directionIncoming)
	}

	return c.quic, nil
}

func (t *QUIC) handleOutgoing(ctx context.Context, q quic.EarlyConnection) (quic.EarlyConnection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, quicConfig.HandshakeIdleTimeout)
	defer openCancel()

	stream, err := q.AcceptStream(openCtx)
	if err != nil {
		return nil, err
	}
	defer WrapQuicConnection(stream, q).Close()

	c, reused, err := t.reuseConnection(ctx, q, stream, directionOutgoing)
	if err != nil {
		return nil, err
	}

	if !reused {
		t.handlePeer(ctx, c.quic, c.peer, directionOutgoing)
	}

	return c.quic, nil
}

func (t *QUIC) handlePeer(ctx context.Context, q quic.EarlyConnection, peer *protocol.Node, dir direction) {
	l := t.Logger.With(
		zap.String("remote", q.RemoteAddr().String()),
		zap.Object("peer", peer),
		zap.String("direction", dir.String()),
		zap.String("key", t.makeCachedKey(peer)),
	)
	l.Debug("Starting goroutines to handle streams and datagrams")
	go t.handleConnection(ctx, q, peer)
	go t.handleDatagram(ctx, q, peer)
	if t.RTTRecorder != nil {
		go t.sendRTTSyn(ctx, q, peer)
	}
	go func(q quic.Connection) {
		<-q.Context().Done()
		l.Debug("Connection with peer closed", zap.Error(q.Context().Err()))
		t.reapPeer(q, peer)
	}(q)
}

func (t *QUIC) background(ctx context.Context) {
	if !t.started.CompareAndSwap(false, true) {
		return
	}
	go t.reaper(ctx)
	go t.handleRTTAck(ctx)
}

func (t *QUIC) AcceptWithListener(ctx context.Context, listener quic.EarlyListener) error {
	t.Logger.Info("Accepting connections", zap.String("listen", listener.Addr().String()))
	t.background(ctx)
	for {
		q, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		go func(q quic.EarlyConnection) {
			if _, err := t.handleIncoming(ctx, q); err != nil {
				if !strings.Contains(err.Error(), reuseErrorState) {
					t.Logger.Error("Incoming connection reuse error", zap.String("endpoint", q.RemoteAddr().String()), zap.Error(err))
				}
				// TODO: figure out a better way to ensure that the peer received cache status before closing
				time.Sleep(time.Second)
				q.CloseWithError(406, err.Error())
			}
		}(q)
	}
}

func (t *QUIC) Accept(ctx context.Context) error {
	if t.ServerTLS == nil {
		return fmt.Errorf("missing ServerTLS")
	}
	l, err := quic.ListenAddrEarly(t.Endpoint.GetAddress(), t.ServerTLS, quicConfig)
	if err != nil {
		return err
	}
	return t.AcceptWithListener(ctx, l)
}

func (t *QUIC) handleDatagram(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	logger := t.Logger.With(zap.String("endpoint", q.RemoteAddr().String()), zap.Object("peer", peer))
	for {
		b, err := q.ReceiveMessage()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				logger.Error("error receiving datagram", zap.Error(err))
			}
			return
		}
		data := &protocol.Datagram{}
		if err := data.UnmarshalVT(b); err != nil {
			logger.Error("error decoding datagram to proto", zap.Error(err))
			continue
		}
		switch data.GetType() {
		case protocol.Datagram_ALIVE:
		case protocol.Datagram_RTT_SYN:
			data.Type = protocol.Datagram_RTT_ACK
			buf, err := data.MarshalVT()
			if err != nil {
				logger.Error("error encoding rtt ack datagram to proto", zap.Error(err))
				continue
			}
			if err := q.SendMessage(buf); err != nil {
				logger.Error("error sending rtt ack datagram", zap.Error(err))
				continue
			}
		case protocol.Datagram_RTT_ACK:
			select {
			case t.rttChan <- &transport.DatagramDelegate{Buffer: data.GetData(), Identity: peer}:
			default:
				logger.Warn("rtt ack buffer full, dropping datagram")
			}
		case protocol.Datagram_DATA:
			select {
			case t.dgramChan <- &transport.DatagramDelegate{Buffer: data.GetData(), Identity: peer}:
			default:
				logger.Warn("data buffer full, dropping datagram")
			}
		default:
			logger.Warn("unknown datagram type: %s", zap.String("type", data.GetType().String()))
		}
	}
}

func (t *QUIC) handleConnection(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	for {
		stream, err := q.AcceptStream(ctx)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				t.Logger.Error("Error accepting new stream from peer", zap.Object("peer", peer), zap.String("remote", q.RemoteAddr().String()), zap.Error(err))
			}
			return
		}
		go t.streamHandler(q, stream, peer)
	}
}

func (t *QUIC) streamHandler(q quic.Connection, stream quic.Stream, peer *protocol.Node) {
	l := t.Logger.With(zap.Object("peer", peer))
	conn := WrapQuicConnection(stream, q)

	var err error
	defer func() {
		if err != nil {
			l.Error("error handshaking on new stream", zap.Error(err))
			conn.Close()
		}
	}()

	rr := protocol.Stream{}
	conn.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.BoundedReceive(conn, &rr, 16)
	if err != nil {
		l.Error("Failed to receive stream handshake", zap.Error(err))
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	if rr.GetType() == protocol.Stream_UNKNOWN_TYPE {
		l.Warn("Received stream with unknown type")
		conn.Close()
		return
	}

	var identity *protocol.Node
	if t.VirtualTransport {
		identity = &protocol.Node{
			Id:      rr.GetTarget().GetId(),
			Address: peer.GetAddress(),
		}
	} else {
		identity = peer
	}

	delegation := &transport.StreamDelegate{
		Identity: identity,
		Conn:     conn,
		Kind:     rr.GetType(),
	}

	if t.UseCertificateIdentity {
		chain := q.ConnectionState().TLS.VerifiedChains
		delegation.Certificate = chain[0][0]
	}

	select {
	case t.streamChan <- delegation:
	default:
		l.Warn("Stream channel full, dropping incoming stream",
			zap.String("kind", rr.GetType().String()),
		)
		conn.Close()
	}
}

func (t *QUIC) Stop() {
	if !t.closed.CompareAndSwap(false, true) {
		return
	}
	t.started.Store(false)
	t.cachedConnections.Range(func(key string, value *nodeConnection) bool {
		value.quic.CloseWithError(0, "Transport closed")
		return true
	})
}
