package sqlite3

import (
	"context"
	"database/sql"

	"go.miragespace.co/specter/spec/chord"
)

func (s *SqliteKV) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		res, err := tx.StmtContext(ctx, s.stmts.prefixAppend).Exec(prefix, child)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return chord.ErrKVPrefixConflict
		}
		return s.updateKeyTracker(ctx, tx, prefix, PrefixFlag, 0)
	})
}

func (s *SqliteKV) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	var one int
	err := s.stmts.prefixContains.QueryRowContext(ctx, prefix, child).Scan(&one)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *SqliteKV) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	rows, err := s.stmts.prefixList.QueryContext(ctx, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([][]byte, 0)
	for rows.Next() {
		var child []byte
		if err := rows.Scan(&child); err != nil {
			return nil, err
		}
		entries = append(entries, child)
	}
	return entries, rows.Err()
}

func (s *SqliteKV) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		_, err := tx.StmtContext(ctx, s.stmts.prefixRemove).Exec(prefix, child)
		if err != nil {
			return err
		}
		return s.updateKeyTracker(ctx, tx, prefix, 0, PrefixFlag)
	})
}
