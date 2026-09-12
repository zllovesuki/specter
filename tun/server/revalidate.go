package server

import (
	"bytes"
	"context"
	"errors"
	"time"

	"go.miragespace.co/specter/util"
)

func (s *Server) maintainSession(ctx context.Context, sess *session) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sess.mu.Lock()
	if sess.state != active || sess.mode != delegated || connectionDone(sess.conn) {
		sess.mu.Unlock()
		return
	}
	sess.revalidateCancel = cancel
	sess.revalidateDone = make(chan struct{})
	defer close(sess.revalidateDone)
	until := sess.authorizedUntil
	sess.mu.Unlock()
	// Shutdown and expiry must close streams even if storage ignores cancellation.
	context.AfterFunc(ctx, func() { sess.close("session maintenance stopped") })
	expiry := time.AfterFunc(time.Until(until), func() {
		sess.mu.Lock()
		expired := sess.state == active && !time.Now().Before(sess.authorizedUntil)
		sess.mu.Unlock()
		if expired {
			sess.close("authorization grace expired")
		}
	})
	defer expiry.Stop()
	// Stagger the first check; subsequent checks stay thirty seconds apart.
	next := time.NewTimer(util.RandomTimeRange(revalidateInterval))
	defer next.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.conn.Done():
			return
		case <-next.C:
			next.Reset(revalidateInterval)
			s.revalidateSession(ctx, sess)
			sess.mu.Lock()
			until, state := sess.authorizedUntil, sess.state
			sess.mu.Unlock()
			if state != active {
				return
			}
			expiry.Reset(time.Until(until))
		}
	}
}

func (s *Server) revalidateSession(ctx context.Context, sess *session) {
	if ctx.Err() != nil {
		return
	}
	start := time.Now()
	sess.mu.Lock()
	if sess.state != active {
		sess.mu.Unlock()
		return
	}
	expired := !start.Before(sess.authorizedUntil)
	sess.mu.Unlock()
	if expired {
		sess.close("authorization grace expired")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	rec, err := s.readDelegation(ctx, sess.grantID)
	if errors.Is(err, errDelegationNotFound) || errors.Is(err, errDelegationInvalid) {
		sess.close(err.Error())
		return
	}
	if err != nil || ctx.Err() != nil {
		return
	}
	expiry := time.Time{}
	if rec.GetExpiresAt() != 0 {
		expiry = time.Unix(rec.GetExpiresAt(), 0)
	}
	if rec.GetHostname() != sess.hostname || !bytes.Equal(rec.GetOwner().GetToken(), sess.owner.GetToken()) || !expiry.Equal(sess.grantExpiry) || (!expiry.IsZero() && !time.Now().Before(expiry)) {
		sess.close("grant authority changed or expired")
		return
	}
	err = s.verifyDelegatedAuthority(ctx, rec)
	if errors.Is(err, errDelegationNotOwned) {
		sess.close(err.Error())
		return
	}
	if err != nil {
		return
	}
	if ctx.Err() == nil {
		sess.extend(start, expiry)
	}
}
