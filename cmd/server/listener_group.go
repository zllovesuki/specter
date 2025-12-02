package server

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"

	cmdlisten "go.miragespace.co/specter/cmd/internal/listen"
	"go.miragespace.co/specter/spec/transport/q"

	"github.com/quic-go/quic-go"
	"go.uber.org/atomic"
)

type multiListener struct {
	listeners []net.Listener
	stop      chan struct{}
	connCh    chan net.Conn
	closeOnce sync.Once
	wg        sync.WaitGroup
	err       atomic.Value
}

func newMultiListener(listeners []net.Listener) net.Listener {
	if len(listeners) == 1 {
		return listeners[0]
	}

	m := &multiListener{
		listeners: listeners,
		stop:      make(chan struct{}),
		connCh:    make(chan net.Conn, len(listeners)),
	}
	for _, l := range listeners {
		m.wg.Add(1)
		go m.serve(l)
	}
	go func() {
		m.wg.Wait()
		close(m.connCh)
	}()
	return m
}

func (m *multiListener) serve(l net.Listener) {
	defer m.wg.Done()
	for {
		conn, err := l.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && m.err.Load() == nil {
				m.err.Store(err)
			}
			return
		}
		select {
		case m.connCh <- conn:
		case <-m.stop:
			conn.Close()
			return
		}
	}
}

func (m *multiListener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-m.connCh:
		if !ok {
			if err, ok := m.err.Load().(error); ok && err != nil {
				return nil, err
			}
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-m.stop:
		return nil, net.ErrClosed
	}
}

func (m *multiListener) Close() error {
	m.closeOnce.Do(func() {
		close(m.stop)
		for _, l := range m.listeners {
			l.Close()
		}
		m.wg.Wait()
	})

	if err, ok := m.err.Load().(error); ok && err != nil {
		return err
	}
	return nil
}

func (m *multiListener) Addr() net.Addr {
	if len(m.listeners) == 0 {
		return nil
	}
	return m.listeners[0].Addr()
}

type multiQuicListener struct {
	listeners []q.Listener
	ctx       context.Context
	cancel    context.CancelFunc
	connCh    chan *quic.Conn
	wg        sync.WaitGroup
	err       atomic.Value
}

func newMultiQuicListener(parent context.Context, listeners []q.Listener) q.Listener {
	if len(listeners) == 1 {
		return listeners[0]
	}

	ctx, cancel := context.WithCancel(parent)
	m := &multiQuicListener{
		listeners: listeners,
		ctx:       ctx,
		cancel:    cancel,
		connCh:    make(chan *quic.Conn, len(listeners)),
	}
	for _, l := range listeners {
		m.wg.Add(1)
		go m.serve(l)
	}
	go func() {
		m.wg.Wait()
		close(m.connCh)
	}()
	return m
}

func (m *multiQuicListener) serve(l q.Listener) {
	defer m.wg.Done()
	for {
		conn, err := l.Accept(m.ctx)
		if err != nil {
			if m.ctx.Err() == nil && !errors.Is(err, net.ErrClosed) && m.err.Load() == nil {
				m.err.Store(err)
			}
			return
		}
		select {
		case m.connCh <- conn:
		case <-m.ctx.Done():
			conn.CloseWithError(0, "listener closed")
			return
		}
	}
}

func (m *multiQuicListener) Accept(ctx context.Context) (*quic.Conn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn, ok := <-m.connCh:
		if !ok {
			if err, ok := m.err.Load().(error); ok && err != nil {
				return nil, err
			}
			return nil, net.ErrClosed
		}
		return conn, nil
	}
}

func (m *multiQuicListener) Close() error {
	m.cancel()
	for _, l := range m.listeners {
		l.Close()
	}
	m.wg.Wait()
	if err, ok := m.err.Load().(error); ok && err != nil {
		return err
	}
	return nil
}

func (m *multiQuicListener) Addr() net.Addr {
	if len(m.listeners) == 0 {
		return nil
	}
	return m.listeners[0].Addr()
}

type udpBinding struct {
	listen     cmdlisten.Address
	packetConn net.PacketConn
	transport  *quic.Transport
}

type multiDialer struct {
	bindings []udpBinding
}

func newMultiDialer(bindings []udpBinding) *multiDialer {
	return &multiDialer{bindings: bindings}
}

func (m *multiDialer) DialEarly(ctx context.Context, addr net.Addr, tlsConf *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	target := cmdlisten.IPAny
	if udpAddr, ok := addr.(*net.UDPAddr); ok && udpAddr.IP != nil {
		if udpAddr.IP.To4() != nil {
			target = cmdlisten.IPV4
		} else if udpAddr.IP.To16() != nil {
			target = cmdlisten.IPV6
		}
	}

	if tr := m.pick(target); tr != nil {
		return tr.DialEarly(ctx, addr, tlsConf, cfg)
	}

	return nil, errors.New("no udp listeners configured for quic transport")
}

func (m *multiDialer) pick(target cmdlisten.IPVersion) *quic.Transport {
	if target != cmdlisten.IPAny {
		for _, b := range m.bindings {
			if b.listen.Version == target {
				return b.transport
			}
		}
	}
	if len(m.bindings) > 0 {
		return m.bindings[0].transport
	}
	return nil
}
