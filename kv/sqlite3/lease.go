package sqlite3

import (
	"context"
	"time"

	"go.miragespace.co/specter/spec/chord"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	err := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		next = uint64(now.Add(ttl).UnixNano())

		// single statement compare-and-set was collaborated with OpenAI GPT o1
		resp := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "owner"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"token": next,
			}),
			Where: clause.Where{
				Exprs: []clause.Expression{
					clause.Lte{
						Column: "token",
						Value:  uint64(now.UnixNano()),
					},
				},
			},
		}).Create(&LeaseEntry{
			Owner: lease,
			Token: next,
		})
		if resp.Error != nil {
			return resp.Error
		}
		if resp.RowsAffected == 0 {
			return chord.ErrKVLeaseConflict
		}
		return s.updateKeyTracker(tx, lease, LeaseFlag, 0)
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
	err := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		next = uint64(time.Now().Add(ttl).UnixNano())

		// single statement compare-and-set was collaborated with OpenAI GPT o1
		resp := tx.Model(&LeaseEntry{}).
			Where(
				"owner = ? AND token = ? AND token > ?",
				lease,
				prevToken,
				uint64(now.UnixNano()),
			).Update("token", next)
		if resp.Error != nil {
			return resp.Error
		}
		if resp.RowsAffected == 0 {
			return chord.ErrKVLeaseExpired
		}
		return s.updateKeyTracker(tx, lease, LeaseFlag, 0)
	})
	if err != nil {
		return 0, err
	}
	// Post-commit check: re-read the row in a new session
	//    If the database row’s token is no longer what we set, it means
	//    some other transaction updated it after we did our compare-and-set.
	var curToken uint64
	readErr := s.writer.WithContext(ctx).
		Model(&LeaseEntry{}).
		Where("owner = ?", lease).
		Pluck("token", &curToken).
		Error
	if readErr != nil {
		// If the row no longer exists or some other error,
		// you could consider that as concurrency override or just return readErr.
		return 0, readErr
	}

	// 3. Compare the token we set (`next`) vs. what's actually in the DB now
	if curToken != next {
		// Another transaction must have updated it post-commit => concurrency lost
		return 0, chord.ErrKVLeaseExpired
	}
	return next, nil
}

func (s *SqliteKV) Release(ctx context.Context, lease []byte, token uint64) error {
	return s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resp := tx.Where("owner = ? AND token = ?", lease, token).
			Delete(&LeaseEntry{})
		if resp.Error != nil {
			return resp.Error
		}
		if resp.RowsAffected == 0 {
			return chord.ErrKVLeaseExpired
		}
		return s.updateKeyTracker(tx, lease, 0, LeaseFlag)
	})
}
