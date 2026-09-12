package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"

	"github.com/twitchtv/twirp"
	"github.com/zhangyunhao116/skipmap"
)

const (
	maxSessions           = 1024
	openFailureCloseDelay = time.Second
)

var (
	revalidateInterval = 30 * time.Second
	revalidateGrace    = 2 * time.Minute
)

type sessionMode uint8

const (
	ephemeral sessionMode = iota
	delegated
)

type sessionState uint8

const (
	provisional sessionState = iota
	active
	terminal
)

type session struct {
	alias            string
	hostname         string
	conn             transport.PhysicalConn
	mode             sessionMode
	spki             []byte
	grantID          string
	owner            *protocol.ClientToken
	grantExpiry      time.Time
	certNotAfter     time.Time
	mu               sync.Mutex
	state            sessionState
	authorizedUntil  time.Time
	revalidateCancel context.CancelFunc
	revalidateDone   chan struct{}
}

type sessionRegistry struct {
	// mu protects admission, byConn, and conditional alias updates.
	mu      sync.Mutex
	byAlias *skipmap.StringMap[*session]
	byConn  map[transport.PhysicalConn]*session
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		byAlias: skipmap.NewString[*session](),
		byConn:  make(map[transport.PhysicalConn]*session),
	}
}

// describe joins operator observations to the exact attachment. Certificate
// identity alone cannot distinguish a stale connection from its replacement.
func (r *sessionRegistry) describe(conn transport.PhysicalConn) (mode, hostname, owner string) {
	if r == nil || conn == nil {
		return "", "", ""
	}
	r.mu.Lock()
	s := r.byConn[conn]
	r.mu.Unlock()
	if s == nil {
		return "", "", ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != active || connectionDone(s.conn) {
		return "", "", ""
	}
	if s.mode == ephemeral {
		return "ephemeral", s.hostname, ""
	}
	return "token", s.hostname, string(s.owner.GetToken())
}

func connectionDone(c transport.PhysicalConn) bool {
	select {
	case <-c.Done():
		return true
	default:
		return false
	}
}

func (r *sessionRegistry) reserve(s *session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byConn[s.conn] != nil {
		return twirp.FailedPrecondition.Error("one Open attempt is allowed per attachment")
	}
	if len(r.byConn) >= maxSessions {
		return twirp.ResourceExhausted.Error("lightweight session limit reached")
	}
	// Failed attempts also occupy this attachment until physical termination.
	r.byConn[s.conn] = s
	go r.watch(s)
	if old, ok := r.byAlias.Load(s.alias); ok {
		old.mu.Lock()
		defer old.mu.Unlock()
		if s.mode == ephemeral && (!bytes.Equal(old.spki, s.spki) || old.hostname != s.hostname) {
			return twirp.PermissionDenied.Error("ephemeral identity collision")
		}
		if old.state != terminal && !connectionDone(old.conn) {
			return twirp.Unavailable.Error("session alias is still attached")
		}
	}
	r.byAlias.Store(s.alias, s)
	return nil
}

func (r *sessionRegistry) watch(s *session) {
	<-s.conn.Done()
	s.mu.Lock()
	s.state = terminal
	cancel := s.revalidateCancel
	done := s.revalidateDone
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		// Keep unfinished storage work in the admission count until it returns.
		<-done
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.byAlias.Load(s.alias); ok && current == s {
		r.byAlias.Delete(s.alias)
	}
	if r.byConn[s.conn] == s {
		delete(r.byConn, s.conn)
	}
}

func (s *session) activate(ctx context.Context, until time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != provisional || connectionDone(s.conn) || ctx.Err() != nil || (s.mode == delegated && !time.Now().Before(until)) {
		return false
	}
	s.state, s.authorizedUntil = active, until
	return true
}

func (s *session) fail(reason string) {
	s.mu.Lock()
	s.state = terminal
	s.mu.Unlock()
	// Allow the unary error response to flush before ending its connection.
	time.AfterFunc(openFailureCloseDelay, func() { s.conn.Close(reason) })
}

func (s *session) close(reason string) {
	s.mu.Lock()
	s.state = terminal
	s.mu.Unlock()
	s.conn.Close(reason)
}

func (r *sessionRegistry) dial(alias, hostname string) (net.Conn, error) {
	s, ok := r.byAlias.Load(alias)
	if !ok {
		return nil, fmt.Errorf("%w: unknown session", transport.ErrNoDirect)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != active || s.hostname != hostname || connectionDone(s.conn) || (s.mode == delegated && !time.Now().Before(s.authorizedUntil)) {
		return nil, fmt.Errorf("%w: session is not authorized", transport.ErrNoDirect)
	}
	conn, err := s.conn.OpenStream(protocol.Stream_DIRECT)
	if err != nil {
		return nil, fmt.Errorf("%w: opening session stream: %v", transport.ErrNoDirect, err)
	}
	return conn, nil
}

func (s *session) authorizationDeadline(start, expiry time.Time) time.Time {
	until := start.Add(revalidateGrace)
	if !expiry.IsZero() && expiry.Before(until) {
		until = expiry
	}
	if s.certNotAfter.Before(until) {
		until = s.certNotAfter
	}
	// Preserve monotonic elapsed-time checks even for wall-clock expiry fields.
	return start.Add(until.Sub(start))
}

func (s *session) extend(start, expiry time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	until := s.authorizationDeadline(start, expiry)
	if s.state == active && time.Now().Before(s.authorizedUntil) && until.After(s.authorizedUntil) {
		s.authorizedUntil = until
	}
}
