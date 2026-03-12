package sqlite3

import (
	"context"
	"database/sql"
	"time"

	"go.miragespace.co/specter/spec/chord"
)

func durationGuard(t time.Duration) (time.Duration, bool) {
	td := t.Truncate(time.Second)
	if td < time.Second {
		return 0, false
	}
	return td, true
}

func (s *SqliteKV) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (uint64, error) {
	ttl, ok := durationGuard(ttl)
	if !ok {
		return 0, chord.ErrKVLeaseInvalidTTL
	}

	var next uint64
	err := withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		now := time.Now()
		next = uint64(now.Add(ttl).UnixNano())

		res, err := tx.StmtContext(ctx, s.stmts.leaseAcquire).Exec(lease, bindUint64AsInt64(next), bindUint64AsInt64(uint64(now.UnixNano())))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return chord.ErrKVLeaseConflict
		}
		return s.updateKeyTracker(ctx, tx, lease, LeaseFlag, 0)
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}

func (s *SqliteKV) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (uint64, error) {
	ttl, ok := durationGuard(ttl)
	if !ok {
		return 0, chord.ErrKVLeaseInvalidTTL
	}

	var next uint64
	err := withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		now := time.Now()
		next = uint64(time.Now().Add(ttl).UnixNano())

		res, err := tx.StmtContext(ctx, s.stmts.leaseRenew).Exec(
			bindUint64AsInt64(next),
			lease,
			bindUint64AsInt64(prevToken),
			bindUint64AsInt64(uint64(now.UnixNano())),
		)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return chord.ErrKVLeaseExpired
		}
		return s.updateKeyTracker(ctx, tx, lease, LeaseFlag, 0)
	})
	if err != nil {
		return 0, err
	}
	// Post-commit check: re-read the row in a new session
	//    If the database row's token is no longer what we set, it means
	//    some other transaction updated it after we did our compare-and-set.
	var curToken int64
	readErr := s.stmts.leaseGet.QueryRowContext(ctx, lease).Scan(&curToken)
	if readErr != nil {
		// If the row no longer exists or some other error,
		// you could consider that as concurrency override or just return readErr.
		return 0, readErr
	}

	// 3. Compare the token we set (`next`) vs. what's actually in the DB now
	if scanInt64AsUint64(curToken) != next {
		// Another transaction must have updated it post-commit => concurrency lost
		return 0, chord.ErrKVLeaseExpired
	}
	return next, nil
}

func (s *SqliteKV) Release(ctx context.Context, lease []byte, token uint64) error {
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		res, err := tx.StmtContext(ctx, s.stmts.leaseRelease).Exec(lease, bindUint64AsInt64(token))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return chord.ErrKVLeaseExpired
		}
		return s.updateKeyTracker(ctx, tx, lease, 0, LeaseFlag)
	})
}
