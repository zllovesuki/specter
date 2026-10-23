package config

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"go.miragespace.co/specter/util"
)

// Store owns one complete local replica. SQLite is authoritative; consumers
// retain their own active configurations.
type Store struct {
	db *sql.DB
	// The commit boundary is replaceable to test ambiguous transaction results.
	commit func(*sql.Tx) error
}

// Open starts an empty replica or recovers an existing one. It never imports
// deployment flags, resets an invalid database, or initializes documents.
func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("configuration directory must not be empty")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := util.MkdirAllDurable(dir, 0700); err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	db, err := openDatabase(dir)
	if err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	s := &Store{db: db, commit: (*sql.Tx).Commit}
	if _, err := s.Snapshot(); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

// Init creates nonempty initial documents only when the store has no documents.
// The check and write share one transaction, including while a server is running.
func Init(dir string, records []Record) (err error) {
	if len(records) == 0 {
		return errors.New("initial configuration must contain at least one document")
	}
	if err := checkRecords(records); err != nil {
		return err
	}
	s, err := Open(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()
	err = s.write(func(tx *sql.Tx) (bool, error) {
		var exists bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM documents)").Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			return false, os.ErrExist
		}
		return writeRecords(tx, records)
	})
	return err
}

// Get returns SQLite's current document, after recovering any uncertain commit.
// Reading does not confirm the durability of a failed save.
func (s *Store) Get(name string) (Record, bool, error) {
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		return Record{}, false, err
	}
	defer conn.Close()
	record, found, err := readRecord(conn, name)
	if err != nil {
		return Record{}, false, discard(conn, err)
	}
	return record, found, nil
}

// Snapshot returns all documents, including unknown schemas, sorted by name.
func (s *Store) Snapshot() ([]Record, error) {
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	records, err := readRecords(conn)
	if err != nil {
		return nil, discard(conn, err)
	}
	return records, nil
}

// Put replaces a complete document after checking this server's current token.
// Disconnected edits can both succeed and later converge by Version ordering.
func (s *Store) Put(name string, schema uint64, value []byte, expectedToken string) (Record, error) {
	var record Record
	err := s.write(func(tx *sql.Tx) (bool, error) {
		current, found, err := readRecord(tx, name)
		if err != nil {
			return false, err
		}
		if current.Token() != expectedToken {
			return false, ErrConflict
		}
		revision := uint64(1)
		if found {
			if schema < current.Version.Schema {
				return false, ErrSchemaRegression
			}
			if current.Version.Revision == math.MaxUint64 {
				return false, errors.New("configuration revision exhausted")
			}
			revision = current.Version.Revision + 1
		}
		record, err = NewRecord(name, schema, revision, value)
		if err != nil {
			return false, err
		}
		return writeRecords(tx, []Record{record})
	})
	return record, err
}

// Merge validates every incoming record and commits all winning versions in one
// transaction. An oversized union is rejected whole, including on retry.
func (s *Store) Merge(records []Record) error {
	if err := checkRecords(records); err != nil {
		return err
	}
	return s.write(func(tx *sql.Tx) (bool, error) {
		return writeRecords(tx, records)
	})
}

// Hold the connection through the commit result so an uncertain connection can
// be retired before another operation uses it. Transactions are never replayed.
func (s *Store) write(change func(*sql.Tx) (bool, error)) error {
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		return discard(conn, err)
	}
	defer tx.Rollback()
	// Reject existing corruption before a winning update could replace it, then
	// validate the complete result before making any change durable.
	_, err = readRecords(tx)
	var changed bool
	if err == nil {
		changed, err = change(tx)
	}
	if err == nil && changed {
		_, err = readRecords(tx)
	}
	if err != nil || !changed {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return discard(conn, errors.Join(err, rollbackErr))
		}
		return err
	}
	if err := s.commit(tx); err != nil {
		// Finish the transaction before retiring its leased connection.
		_ = tx.Rollback()
		return discard(conn, err)
	}
	return nil
}

func discard(conn *sql.Conn, cause error) error {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	return fmt.Errorf("%w: %w", ErrDurabilityUncertain, cause)
}

func (s *Store) Close() error {
	return s.db.Close()
}
