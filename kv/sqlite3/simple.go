package sqlite3

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *SqliteKV) Put(ctx context.Context, key []byte, value []byte) error {
	entry := &SimpleEntry{
		Key:   key,
		Value: value,
	}

	return s.writer.
		WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value"}),
			}).Create(entry).Error; err != nil {
				return err
			}
			return s.updateKeyTracker(tx, key, SimpleFlag, 0)
		})
}

func (s *SqliteKV) Get(ctx context.Context, key []byte) ([]byte, error) {
	entry := &SimpleEntry{
		Key: key,
	}
	tx := s.reader.WithContext(ctx)
	resp := tx.Select("value").Take(entry)
	if resp.Error != nil {
		if errors.Is(resp.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, resp.Error
	}
	// errata: gorm library doesn't distinguish between nil and empty byte slice
	if entry.Value == nil {
		entry.Value = []byte{}
	}
	return entry.Value, nil
}

func (s *SqliteKV) Delete(ctx context.Context, key []byte) error {
	entry := &SimpleEntry{
		Key: key,
	}

	return s.writer.
		WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(entry).Error; err != nil {
				return err
			}
			return s.updateKeyTracker(tx, key, 0, SimpleFlag)
		})
}
