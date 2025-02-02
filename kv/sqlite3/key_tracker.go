package sqlite3

import (
	"errors"

	"go.miragespace.co/specter/spec/chord"

	"gorm.io/gorm"
)

const (
	SimpleFlag = 1 << iota
	PrefixFlag
	LeaseFlag
)

type KeyTracker struct {
	Key   []byte `gorm:"primaryKey"`
	Hash  uint64 `gorm:"index:idx_hash,sort:asc"`
	Flags uint8  // Bitmask: 1=Simple, 2=Prefix, 4=Lease
}

// optimized by GPT 4o-mini via flags
func (s *SqliteKV) updateKeyTracker(tx *gorm.DB, key []byte, addFlags, removeFlags uint8) error {
	var (
		tracker KeyTracker
	)

	// Fetch existing tracker
	lookupErr := tx.Take(&tracker, "key = ?", key).Error
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return lookupErr
	}

	// consistency check: if this instance has a different hash function, refuse to modify
	if lookupErr == nil && tracker.Hash != s.hashFn(key) {
		return chord.ErrKVHashFnChanged
	}

	// If tracker does not exist but we're adding flags, create a new entry
	if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		if addFlags != 0 {
			newTracker := KeyTracker{
				Key:   key,
				Hash:  s.hashFn(key),
				Flags: addFlags,
			}
			return tx.Create(&newTracker).Error
		}
		return nil
	}

	// Check for remaining values before removing flags
	if removeFlags&PrefixFlag != 0 {
		var count int64
		if err := tx.Model(&PrefixEntry{}).Where("prefix = ?", key).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			removeFlags &^= PrefixFlag // Cancel removing PrefixFlag
		}
	}

	// Update bitmask flags
	tracker.Flags = (tracker.Flags | addFlags) &^ removeFlags

	// If no values remain, delete tracker
	if tracker.Flags == 0 {
		return tx.Delete(&tracker).Error
	}

	// Otherwise, update tracker
	return tx.Save(&tracker).Error
}
