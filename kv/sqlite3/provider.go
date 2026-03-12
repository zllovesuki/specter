package sqlite3

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"

	"go.miragespace.co/specter/spec/protocol"
)

const removeKeysBatchSize = 200

func importedSimpleValue(val *protocol.KVTransfer) ([]byte, bool) {
	if val == nil {
		return nil, false
	}
	if val.SimpleValue != nil {
		return val.SimpleValue, true
	}
	if len(val.PrefixChildren) == 0 && val.LeaseToken == 0 {
		return []byte{}, true
	}
	return nil, false
}

func (s *SqliteKV) ListKeys(ctx context.Context, prefix []byte) ([]*protocol.KeyComposite, error) {
	keys := make([]*protocol.KeyComposite, 0)

	rows, err := s.stmts.listKeys.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			key   []byte
			flags uint8
		)
		if err := rows.Scan(&key, &flags); err != nil {
			return nil, err
		}
		if !bytes.HasPrefix(key, prefix) {
			continue
		}
		if flags&SimpleFlag != 0 {
			keys = append(keys, &protocol.KeyComposite{
				Type: protocol.KeyComposite_SIMPLE,
				Key:  key,
			})
		}
		if flags&PrefixFlag != 0 {
			keys = append(keys, &protocol.KeyComposite{
				Type: protocol.KeyComposite_PREFIX,
				Key:  key,
			})
		}
		if flags&LeaseFlag != 0 {
			keys = append(keys, &protocol.KeyComposite{
				Type: protocol.KeyComposite_LEASE,
				Key:  key,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return keys, nil
}

func (s *SqliteKV) Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error {
	if len(keys) != len(values) {
		return fmt.Errorf("keys and values length mismatch: %d != %d", len(keys), len(values))
	}
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		for i, key := range keys {
			var (
				val        = values[i]
				flag uint8 = 0
			)

			if val == nil {
				return fmt.Errorf("values[%d] is nil", i)
			}

			if simpleValue, ok := importedSimpleValue(val); ok {
				_, err := tx.StmtContext(ctx, s.stmts.simplePut).Exec(key, simpleValue)
				if err != nil {
					return err
				}
				flag |= SimpleFlag
			}

			// there's no point to override prefix values, but we need to handle conflict
			children := val.GetPrefixChildren()
			for _, child := range children {
				_, err := tx.StmtContext(ctx, s.stmts.prefixAppend).Exec(key, child)
				if err != nil {
					return err
				}
			}
			if len(children) > 0 {
				flag |= PrefixFlag
			}

			// override lease token when importing
			if val.GetLeaseToken() != 0 {
				_, err := tx.StmtContext(ctx, s.stmts.leaseImport).Exec(key, bindUint64AsInt64(val.GetLeaseToken()))
				if err != nil {
					return err
				}
				flag |= LeaseFlag
			}
			if err := s.updateKeyTracker(ctx, tx, key, flag, 0); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SqliteKV) Export(ctx context.Context, keys [][]byte) ([]*protocol.KVTransfer, error) {
	vals := make([]*protocol.KVTransfer, len(keys))
	err := withReadTx(ctx, s.reader, func(tx *sql.Tx) error {
		for i, key := range keys {
			var (
				simpleValue []byte
				prefix      [][]byte
				leaseToken  int64
			)

			// simple value
			simpleErr := tx.StmtContext(ctx, s.stmts.exportSimpleGet).QueryRow(key).Scan(&simpleValue)
			if simpleErr != nil && simpleErr != sql.ErrNoRows {
				return simpleErr
			}
			// Preserve behavior: distinguish between missing row (nil) and empty blob
			if simpleErr == nil && simpleValue == nil {
				simpleValue = []byte{}
			}

			// prefix children
			prefixRows, err := tx.StmtContext(ctx, s.stmts.exportPrefixList).Query(key)
			if err != nil {
				return err
			}
			prefix = make([][]byte, 0)
			for prefixRows.Next() {
				var child []byte
				if err := prefixRows.Scan(&child); err != nil {
					prefixRows.Close()
					return err
				}
				prefix = append(prefix, child)
			}
			if err := prefixRows.Err(); err != nil {
				prefixRows.Close()
				return err
			}
			prefixRows.Close()

			// lease token
			leaseErr := tx.StmtContext(ctx, s.stmts.exportLeaseGet).QueryRow(key).Scan(&leaseToken)
			if leaseErr != nil && leaseErr != sql.ErrNoRows {
				return leaseErr
			}

			val := &protocol.KVTransfer{
				SimpleValue:    simpleValue,
				PrefixChildren: prefix,
				LeaseToken:     scanInt64AsUint64(leaseToken),
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
	if len(keys) == 0 {
		return nil
	}
	return withWriteTx(ctx, s.writer, func(tx *sql.Tx) error {
		for start := 0; start < len(keys); start += removeKeysBatchSize {
			end := min(start+removeKeysBatchSize, len(keys))
			batch := keys[start:end]
			ph := placeholders(len(batch))
			args := make([]any, len(batch))
			for i, k := range batch {
				args[i] = k
			}

			if _, err := tx.Exec("DELETE FROM `simple_entries` WHERE `key` IN ("+ph+")", args...); err != nil {
				return err
			}
			if _, err := tx.Exec("DELETE FROM `prefix_entries` WHERE `prefix` IN ("+ph+")", args...); err != nil {
				return err
			}
			if _, err := tx.Exec("DELETE FROM `lease_entries` WHERE `owner` IN ("+ph+")", args...); err != nil {
				return err
			}
			if _, err := tx.Exec("DELETE FROM `key_trackers` WHERE `key` IN ("+ph+")", args...); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SqliteKV) RangeKeys(ctx context.Context, low uint64, high uint64) ([][]byte, error) {
	keys := make([][]byte, 0)

	var args []any

	lowI := bindUint64AsInt64(low)
	highI := bindUint64AsInt64(high)

	var stmt *sql.Stmt
	if high > low {
		stmt = s.stmts.rangeKeysNorm
	} else {
		stmt = s.stmts.rangeKeysWrap
	}
	args = []any{lowI, highI, highI}

	rows, err := stmt.QueryContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var key []byte
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
