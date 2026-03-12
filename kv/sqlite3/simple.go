package sqlite3

import (
	"context"
	"database/sql"
)

func (s *SqliteKV) Put(ctx context.Context, key []byte, value []byte) error {
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		_, err := tx.StmtContext(ctx, s.stmts.simplePut).Exec(key, value)
		if err != nil {
			return err
		}
		return s.updateKeyTracker(ctx, tx, key, SimpleFlag, 0)
	})
}

func (s *SqliteKV) Get(ctx context.Context, key []byte) ([]byte, error) {
	var value []byte
	err := s.stmts.simpleGet.QueryRowContext(ctx, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	// Preserve behavior: distinguish between NULL (missing) and empty blob
	if value == nil {
		value = []byte{}
	}
	return value, nil
}

func (s *SqliteKV) Delete(ctx context.Context, key []byte) error {
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		_, err := tx.StmtContext(ctx, s.stmts.simpleDel).Exec(key)
		if err != nil {
			return err
		}
		return s.updateKeyTracker(ctx, tx, key, 0, SimpleFlag)
	})
}
