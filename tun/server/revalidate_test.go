package server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func revalidationFixture(t *testing.T, count int) (*Server, *mocks.VNode, []*session) {
	t.Helper()
	n := new(mocks.VNode)
	s := &Server{
		Config: Config{
			ParentContext: t.Context(),
			Chord:         n,
		},
		sessions: newSessionRegistry(),
	}
	sessions := make([]*session, 0, count)
	for i := 0; i < count; i++ {
		pc := mocks.NewPhysicalConn(func(*transport.StreamDelegate) {})
		t.Cleanup(func() { pc.Close("test cleanup") })
		sess := &session{
			alias:        tun.SessionAlias([16]byte{byte(i + 1)}),
			hostname:     "test",
			conn:         pc,
			mode:         delegated,
			grantID:      tun.DelegationID([32]byte{2}),
			owner:        &protocol.ClientToken{Token: []byte("owner")},
			certNotAfter: time.Now().Add(time.Hour),
		}
		require.NoError(t, s.sessions.reserve(sess))
		sessions = append(sessions, sess)
	}
	return s, n, sessions
}

func revalidationRecord(t *testing.T, sess *session) []byte {
	t.Helper()
	rec := &protocol.DelegationRecord{
		Version:  1,
		Id:       sess.grantID,
		Hostname: sess.hostname,
		Owner:    sess.owner,
	}
	if !sess.grantExpiry.IsZero() {
		rec.ExpiresAt = sess.grantExpiry.Unix()
	}
	data, err := rec.MarshalVT()
	require.NoError(t, err)
	return data
}

func TestRevalidationIndependentSessions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, n, sessions := revalidationFixture(t, 3)
		// All three connections share a token; each must maintain its own authority.
		n.On("Get", mock.Anything, []byte(tun.DelegationKey(sessions[0].grantID))).
			Run(func(mock.Arguments) { time.Sleep(2 * time.Second) }).
			Return(revalidationRecord(t, sessions[0]), nil)
		n.On("PrefixContains", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
		until := time.Now().Add(revalidateGrace)
		for _, sess := range sessions {
			require.True(t, sess.activate(t.Context(), until))
			go s.maintainSession(s.ParentContext, sess)
		}
		time.Sleep(revalidateInterval + lookupTimeout)
		synctest.Wait()
		for i, sess := range sessions {
			sess.mu.Lock()
			extended := sess.authorizedUntil.After(until)
			sess.mu.Unlock()
			require.True(t, extended, "session %d was starved", i)
		}
		sessions[0].conn.Close("one connection ended")
		// Successful checks must keep resetting expiry without affecting siblings.
		time.Sleep(2 * revalidateGrace)
		synctest.Wait()
		for _, sess := range sessions[1:] {
			require.False(t, connectionDone(sess.conn))
			sess.mu.Lock()
			authorized := time.Now().Before(sess.authorizedUntil)
			sess.mu.Unlock()
			require.True(t, authorized)
		}
		n.AssertExpectations(t)
	})
}

func TestRevalidationExpiryDuringBlockedRead(t *testing.T) {
	for _, limit := range []string{"grace", "grant", "certificate"} {
		t.Run(limit, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, n, sessions := revalidationFixture(t, 1)
				sess := sessions[0]
				switch limit {
				case "grant":
					sess.grantExpiry = time.Now().Add(time.Minute)
				case "certificate":
					sess.certNotAfter = time.Now().Add(time.Minute)
				}
				until := sess.authorizationDeadline(time.Now(), sess.grantExpiry)
				require.True(t, sess.activate(t.Context(), until))
				stream, err := sess.conn.OpenStream(protocol.Stream_DIRECT)
				require.NoError(t, err)
				defer stream.Close()
				started := make(chan context.Context, 1)
				n.On("Get", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					started <- args.Get(0).(context.Context)
					time.Sleep(revalidateGrace) // Deliberately ignore the lookup deadline.
				}).Return(revalidationRecord(t, sess), nil).Once()
				done := make(chan struct{})
				go func() {
					defer close(done)
					s.maintainSession(s.ParentContext, sess)
				}()
				readCtx := <-started
				time.Sleep(time.Until(until))
				synctest.Wait()
				require.True(t, connectionDone(sess.conn))
				require.Error(t, readCtx.Err())
				s.sessions.mu.Lock()
				pending := len(s.sessions.byConn)
				s.sessions.mu.Unlock()
				require.Equal(t, 1, pending, "unfinished reads must retain their admission slot")
				_, err = stream.Read(make([]byte, 1))
				require.Error(t, err, "expiry must close existing streams")
				<-done // Let the late successful read return; it must not revive authority.
				synctest.Wait()
				s.sessions.mu.Lock()
				pending = len(s.sessions.byConn)
				s.sessions.mu.Unlock()
				require.Zero(t, pending)
				sess.mu.Lock()
				state, finalDeadline := sess.state, sess.authorizedUntil
				sess.mu.Unlock()
				require.Equal(t, terminal, state)
				require.Equal(t, until, finalDeadline)
				n.AssertNotCalled(t, "PrefixContains", mock.Anything, mock.Anything, mock.Anything)
				n.AssertExpectations(t)
			})
		})
	}
}

func TestRevalidationStopsWithSession(t *testing.T) {
	for _, reason := range []string{"disconnect", "shutdown"} {
		for _, pendingRead := range []bool{false, true} {
			name := reason + "/idle"
			if pendingRead {
				name = reason + "/lookup"
			}
			t.Run(name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					s, n, sessions := revalidationFixture(t, 1)
					sess := sessions[0]
					require.True(t, sess.activate(t.Context(), time.Now().Add(revalidateGrace)))
					ctx, cancel := context.WithCancel(s.ParentContext)
					defer cancel()
					started := make(chan context.Context, 1)
					if pendingRead {
						n.On("Get", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
							readCtx := args.Get(0).(context.Context)
							started <- readCtx
							<-readCtx.Done()
						}).Return(nil, context.Canceled).Once()
					}
					done := make(chan struct{})
					go func() {
						defer close(done)
						s.maintainSession(ctx, sess)
					}()
					synctest.Wait()
					var readCtx context.Context
					if pendingRead {
						readCtx = <-started
					}
					if reason == "disconnect" {
						sess.conn.Close("disconnect")
					} else {
						cancel()
					}
					synctest.Wait()
					select {
					case <-done:
					default:
						t.Fatal("session worker did not stop")
					}
					require.True(t, connectionDone(sess.conn))
					if pendingRead {
						require.ErrorIs(t, readCtx.Err(), context.Canceled)
					}
					s.sessions.mu.Lock()
					remaining := len(s.sessions.byConn)
					s.sessions.mu.Unlock()
					require.Zero(t, remaining)
					n.AssertExpectations(t)
				})
			})
		}
	}
}

func TestRevalidation(t *testing.T) {
	for _, scenario := range []string{"transient", "revoked", "delayed success"} {
		t.Run(scenario, func(t *testing.T) {
			s, n, _, cert := sessionFixture(t)
			ctx, pc := sessionContext(t, cert)
			sess := &session{
				alias:        tun.SessionAlias([16]byte{1}),
				hostname:     "test",
				conn:         pc,
				mode:         delegated,
				grantID:      tun.DelegationID([32]byte{2}),
				owner:        &protocol.ClientToken{Token: []byte("owner")},
				certNotAfter: time.Now().Add(time.Hour),
			}
			require.NoError(t, s.sessions.reserve(sess))
			until := time.Now().Add(40 * time.Millisecond)
			require.True(t, sess.activate(ctx, until))
			switch scenario {
			case "transient":
				n.On("Get", mock.Anything, mock.Anything).Return(nil, errors.New("temporary failure")).Once()
				s.revalidateSession(ctx, sess)
				require.Equal(t, until, sess.authorizedUntil)
				require.False(t, connectionDone(pc))
				time.Sleep(time.Until(until) + time.Millisecond)
				_, err := s.sessions.dial(sess.alias, sess.hostname)
				require.ErrorIs(t, err, transport.ErrNoDirect)
				s.revalidateSession(ctx, sess)
			case "revoked":
				n.On("Get", mock.Anything, mock.Anything).Return(nil, nil).Once()
				s.revalidateSession(ctx, sess)
			case "delayed success":
				rec := &protocol.DelegationRecord{
					Version:  1,
					Id:       sess.grantID,
					Hostname: sess.hostname,
					Owner:    sess.owner,
				}
				data, _ := rec.MarshalVT()
				n.On("Get", mock.Anything, mock.Anything).Run(func(mock.Arguments) { time.Sleep(10 * time.Millisecond) }).Return(data, nil).Once()
				n.On("PrefixContains", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()
				start := time.Now()
				s.revalidateSession(ctx, sess)
				require.True(t, sess.authorizedUntil.Before(start.Add(revalidateGrace+5*time.Millisecond)))
				sess.close("test close")
				previous := sess.authorizedUntil
				sess.extend(time.Now().Add(time.Hour), time.Time{})
				require.Equal(t, previous, sess.authorizedUntil)
			}
			require.True(t, connectionDone(pc))
			n.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
			n.AssertExpectations(t)
		})
	}
}
