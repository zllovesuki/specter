package sqlite3

import (
	"bytes"
	"context"
	"errors"

	"go.miragespace.co/specter/spec/protocol"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *SqliteKV) ListKeys(ctx context.Context, prefix []byte) ([]*protocol.KeyComposite, error) {
	keys := make([]*protocol.KeyComposite, 0)
	batches := make([]KeyTracker, 0)

	err := s.reader.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&KeyTracker{}).FindInBatches(&batches, 50, func(tx *gorm.DB, batch int) error {
			for _, tracker := range batches {
				if !bytes.HasPrefix(tracker.Key, prefix) {
					continue
				}
				if tracker.Flags&SimpleFlag != 0 {
					keys = append(keys, &protocol.KeyComposite{
						Type: protocol.KeyComposite_SIMPLE,
						Key:  tracker.Key,
					})
				}
				if tracker.Flags&PrefixFlag != 0 {
					keys = append(keys, &protocol.KeyComposite{
						Type: protocol.KeyComposite_PREFIX,
						Key:  tracker.Key,
					})
				}
				if tracker.Flags&LeaseFlag != 0 {
					keys = append(keys, &protocol.KeyComposite{
						Type: protocol.KeyComposite_LEASE,
						Key:  tracker.Key,
					})
				}
			}
			return nil
		}).Error
	})
	if err != nil {
		return nil, err
	}

	return keys, nil
}

func (s *SqliteKV) Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error {
	err := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, key := range keys {
			var (
				val        = values[i]
				flag uint8 = 0
			)

			// override simple value when importing
			if len(val.GetSimpleValue()) > 0 {
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "key"}},
					DoUpdates: clause.AssignmentColumns([]string{"value"}),
				}).Create(&SimpleEntry{
					Key:   key,
					Value: val.GetSimpleValue(),
				}).Error; err != nil {
					return err
				}
				flag |= SimpleFlag
			}

			// there's no point to override prefix values, but we need to handle conflict
			children := val.GetPrefixChildren()
			if len(children) > 0 {
				entries := make([]PrefixEntry, len(children))
				for k, val := range children {
					entries[k] = PrefixEntry{
						Prefix: key,
						Child:  val,
					}
				}
				if err := tx.Clauses(clause.OnConflict{
					DoNothing: true,
				}).Create(entries).Error; err != nil {
					return err
				}
				flag |= PrefixFlag
			}

			// override lease token when importing
			if val.GetLeaseToken() != 0 {
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "owner"}},
					DoUpdates: clause.AssignmentColumns([]string{"token"}),
				}).Create(&LeaseEntry{
					Owner: key,
					Token: val.GetLeaseToken(),
				}).Error; err != nil {
					return err
				}
				flag |= LeaseFlag
			}
			if err := s.updateKeyTracker(tx, key, flag, 0); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SqliteKV) Export(ctx context.Context, keys [][]byte) ([]*protocol.KVTransfer, error) {
	vals := make([]*protocol.KVTransfer, len(keys))
	err := s.reader.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, key := range keys {
			var (
				simple SimpleEntry
				prefix [][]byte
				lease  LeaseEntry
			)

			simpleErr := tx.Where("key = ?", key).Take(&simple).Error
			prefixErr := tx.Model(&PrefixEntry{}).Select("child").Where("prefix = ?", key).Find(&prefix).Error
			leaseErr := tx.Where("owner = ?", key).Take(&lease).Error

			if !errors.Is(simpleErr, gorm.ErrRecordNotFound) && simpleErr != nil {
				return simpleErr
			}
			if !errors.Is(prefixErr, gorm.ErrRecordNotFound) && prefixErr != nil {
				return prefixErr
			}
			if !errors.Is(leaseErr, gorm.ErrRecordNotFound) && leaseErr != nil {
				return leaseErr
			}

			val := &protocol.KVTransfer{
				SimpleValue:    simple.Value,
				PrefixChildren: prefix,
				LeaseToken:     lease.Token,
			}
			vals[i] = val
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return vals, nil
}

func (s *SqliteKV) RemoveKeys(ctx context.Context, keys [][]byte) error {
	return s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&SimpleEntry{}, keys).Error; err != nil {
			return err
		}
		if err := tx.Delete(&PrefixEntry{}, "prefix in ?", keys).Error; err != nil {
			return err
		}
		if err := tx.Delete(&LeaseEntry{}, keys).Error; err != nil {
			return err
		}
		return tx.Delete(&KeyTracker{}, keys).Error
	})
}

func (s *SqliteKV) RangeKeys(ctx context.Context, low uint64, high uint64) ([][]byte, error) {
	keys := make([][]byte, 0)

	var cond *gorm.DB
	tx := s.reader.WithContext(ctx)

	if high > low {
		cond = tx.Model(&KeyTracker{}).
			Where(
				tx.Where("? < hash", low),
				tx.Where("hash < ?", high),
			).
			Or(
				tx.Where("hash = ?", high), // inclusive
			)
	} else {
		cond = tx.Model(&KeyTracker{}).
			Where("? < hash", low).
			Or("hash < ?", high).
			Or("hash = ?", high) // inclusive
	}

	if err := cond.Select("key").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}
