package sqlite3

import (
	"context"
	"database/sql"
	"strings"
)

// withWriteTx executes fn within a write transaction.
func withWriteTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// withReadTx executes fn within a read-only transaction.
func withReadTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// placeholders returns a comma-separated string of n "?" placeholders.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// bindUint64AsInt64 converts a uint64 to int64 for SQLite INTEGER binding.
// SQLite stores all integers as signed 64-bit. Values derived from
// chord.Hash (48-bit) and lease tokens (positive UnixNano) fit safely.
func bindUint64AsInt64(v uint64) int64 {
	return int64(v)
}

// scanInt64AsUint64 converts a scanned int64 back to uint64.
func scanInt64AsUint64(v int64) uint64 {
	return uint64(v)
}
