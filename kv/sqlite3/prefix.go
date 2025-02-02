package sqlite3

import (
	"context"

	"go.miragespace.co/specter/spec/chord"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *SqliteKV) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	entry := &PrefixEntry{
		Prefix: prefix,
		Child:  child,
	}

	return s.writer.
		WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			resp := tx.Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(entry)
			if resp.Error != nil {
				return resp.Error
			}
			if resp.RowsAffected == 0 {
				return chord.ErrKVPrefixConflict
			}
			return s.updateKeyTracker(tx, prefix, PrefixFlag, 0)
		})
}

func (s *SqliteKV) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	entry := &PrefixEntry{
		Prefix: prefix,
		Child:  child,
	}

	tx := s.reader.WithContext(ctx)
	resp := tx.Limit(1).Find(entry)
	if resp.Error != nil {
		return false, resp.Error
	}

	return resp.RowsAffected > 0, nil
}

func (s *SqliteKV) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	entries := make([][]byte, 0)

	tx := s.reader.WithContext(ctx)
	resp := tx.Model(&PrefixEntry{}).Select("child").Where("prefix = ?", prefix).Find(&entries)
	if resp.Error != nil {
		return nil, resp.Error
	}

	return entries, nil
}

func (s *SqliteKV) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	entry := &PrefixEntry{
		Prefix: prefix,
		Child:  child,
	}

	return s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(entry).Error; err != nil {
			return err
		}
		return s.updateKeyTracker(tx, prefix, 0, PrefixFlag)
	})
}
