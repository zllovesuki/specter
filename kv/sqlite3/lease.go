package sqlite3

import (
	"context"
	"database/sql"
	"time"

	leaseToken "go.miragespace.co/specter/kv/internal/lease"
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
		var err error
		next, err = leaseToken.Next(now, ttl, 0)
		if err != nil {
			return err
		}

		res, err := tx.StmtContext(ctx, s.stmts.leaseAcquire).Exec(bindBytesAsBlob(lease), bindUint64AsInt64(next), bindUint64AsInt64(uint64(now.UnixNano())))
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
		var err error
		next, err = leaseToken.Next(now, ttl, prevToken)
		if err != nil {
			return err
		}

		res, err := tx.StmtContext(ctx, s.stmts.leaseRenew).Exec(
			bindUint64AsInt64(next),
			bindBytesAsBlob(lease),
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
	return next, nil
}

func (s *SqliteKV) Release(ctx context.Context, lease []byte, token uint64) error {
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		res, err := tx.StmtContext(ctx, s.stmts.leaseRelease).Exec(bindBytesAsBlob(lease), bindUint64AsInt64(token))
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
