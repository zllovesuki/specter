package sqlite3

import (
	"context"
	"database/sql"

	"go.miragespace.co/specter/spec/chord"
)

const (
	SimpleFlag = 1 << iota
	PrefixFlag
	LeaseFlag
)

type KeyTracker struct {
	Key   []byte
	Hash  uint64
	Flags uint8 // Bitmask: 1=Simple, 2=Prefix, 4=Lease
}

// optimized by GPT 4o-mini via flags
func (s *SqliteKV) updateKeyTracker(ctx context.Context, tx *sql.Tx, key []byte, addFlags, removeFlags uint8) error {
	var (
		hash  int64
		flags uint8
	)

	// Fetch existing tracker
	lookupErr := tx.StmtContext(ctx, s.stmts.trackerLookup).QueryRow(key).Scan(&hash, &flags)
	if lookupErr != nil && lookupErr != sql.ErrNoRows {
		return lookupErr
	}

	found := lookupErr == nil

	// consistency check: if this instance has a different hash function, refuse to modify
	if found && scanInt64AsUint64(hash) != s.hashFn(key) {
		return chord.ErrKVHashFnChanged
	}

	// If tracker does not exist but we're adding flags, create a new entry
	if !found {
		if addFlags != 0 {
			_, err := tx.StmtContext(ctx, s.stmts.trackerInsert).Exec(key, bindUint64AsInt64(s.hashFn(key)), addFlags)
			return err
		}
		return nil
	}

	// Check for remaining values before removing flags
	if removeFlags&PrefixFlag != 0 {
		var count int64
		if err := tx.StmtContext(ctx, s.stmts.prefixCount).QueryRow(key).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			removeFlags &^= PrefixFlag // Cancel removing PrefixFlag
		}
	}

	// Update bitmask flags
	newFlags := (flags | addFlags) &^ removeFlags

	// If no values remain, delete tracker
	if newFlags == 0 {
		_, err := tx.StmtContext(ctx, s.stmts.trackerDelete).Exec(key)
		return err
	}

	// Otherwise, update tracker
	_, err := tx.StmtContext(ctx, s.stmts.trackerUpdate).Exec(newFlags, key)
	return err
}
