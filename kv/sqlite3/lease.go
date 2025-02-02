package sqlite3

import (
	"context"
	"errors"
	"time"

	"go.miragespace.co/specter/spec/chord"

	"gorm.io/gorm"
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
	entry := &LeaseEntry{
		Owner: lease,
	}

	err := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if resp := tx.Where(LeaseEntry{
			Owner: lease,
		}).Attrs(LeaseEntry{
			Token: 0,
		}).FirstOrCreate(&entry); resp.Error != nil {
			return resp.Error
		}
		curr := entry.Token
		ref := time.Now()
		if curr > uint64(ref.UnixNano()) {
			return chord.ErrKVLeaseConflict
		}
		next = uint64(ref.Add(ttl).UnixNano())
		resp := tx.Model(&LeaseEntry{}).Where(LeaseEntry{
			Owner: lease,
			Token: curr,
		}).Update("token", next)
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
	entry := &LeaseEntry{
		Owner: lease,
	}
	err := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Select("token").Take(entry).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return chord.ErrKVLeaseExpired
			}
			return err
		}

		now := time.Now()
		next = uint64(time.Now().Add(ttl).UnixNano())

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
