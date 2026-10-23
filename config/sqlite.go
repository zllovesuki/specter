package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"go.miragespace.co/specter/util/sqlite"

	"github.com/ncruces/go-sqlite3"
)

const (
	FileName        = "db"
	applicationID   = 0x53504346 // SPCF
	databaseVersion = 1
)

const schema = `
CREATE TABLE documents (
 name TEXT PRIMARY KEY NOT NULL,
 value BLOB NOT NULL,
 version BLOB NOT NULL CHECK(length(version) = 48)
) STRICT;
PRAGMA application_id = 1397769030;
PRAGMA user_version = 1;
`

func openDatabase(dir string) (*sql.DB, error) {
	path := filepath.Join(dir, FileName)
	// SQLite's default file mode follows the process umask. Create privately,
	// accepting another opener's creation before SQLite serializes schema setup.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		if err := f.Close(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("configuration database must be a regular file")
	}
	// Replacement connections must not recreate a missing file with default
	// permissions. The store owns creation; SQLite opens the existing database.
	uri := url.URL{Scheme: "file", OmitHost: true, Path: filepath.ToSlash(path)}
	uri.RawQuery = url.Values{
		"mode":    {"rw"},
		"_txlock": {"immediate"},
		"_pragma": {"busy_timeout(5000)"},
	}.Encode()
	db, err := sqlite.Open(uri.String(), setupDatabase)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// Connection setup runs for initial and replacement connections. The small
// SQLite-specific bootstrap precedes ordinary database/sql queries.
func setupDatabase(conn *sqlite3.Conn) error {
	// SQLite rejects an oversized stored row before copying it into Go memory.
	conn.Limit(sqlite3.LIMIT_LENGTH, MaxValueBytes+MaxNameBytes+versionBytes+4096)
	// The URI installs the busy handler before any database inspection.
	if err := conn.Exec("PRAGMA synchronous=EXTRA; BEGIN IMMEDIATE"); err != nil {
		return err
	}
	id, err := queryInt(conn, "PRAGMA application_id")
	if err != nil {
		return err
	}
	version, err := queryInt(conn, "PRAGMA user_version")
	if err != nil {
		return err
	}
	// An interrupted first transaction can leave an empty SQLite database.
	// Only its pristine state may acquire our schema; foreign files stay intact.
	pristine := id == 0 && version == 0
	if pristine {
		objects, err := queryInt(conn, "SELECT COUNT(*) FROM sqlite_schema")
		if err != nil {
			return err
		}
		pristine = objects == 0
	}
	if !pristine && (id != applicationID || version != databaseVersion) {
		return fmt.Errorf("unsupported configuration database (application %d, version %d)", id, version)
	}
	if pristine {
		if err := conn.Exec(schema); err != nil {
			return err
		}
	}
	return conn.Exec("COMMIT; PRAGMA journal_mode=DELETE")
}

func queryInt(conn *sqlite3.Conn, query string) (int64, error) {
	stmt, _, err := conn.Prepare(query)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	if !stmt.Step() {
		return 0, errors.Join(errors.New("missing configuration database metadata"), stmt.Err())
	}
	if stmt.ColumnType(0) != sqlite3.INTEGER {
		return 0, errors.New("invalid configuration database metadata")
	}
	return stmt.ColumnInt64(0), nil
}

func readRecord(query queryer, name string) (Record, bool, error) {
	row := query.QueryRowContext(context.Background(), "SELECT name, value, version FROM documents WHERE name=?", name)
	record, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	return record, err == nil, err
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readRecords(query queryer) ([]Record, error) {
	rows, err := query.QueryContext(context.Background(), "SELECT name, value, version FROM documents ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Record, 0)
	size := len(`{"format":1,"records":[]}`)
	for rows.Next() {
		if len(records) == MaxDocuments {
			return nil, ErrTooLarge
		}
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		size += len(encoded)
		if len(records) != 0 {
			size++
		}
		if size > MaxSnapshotBytes {
			return nil, ErrTooLarge
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func scanRecord(row interface{ Scan(...any) error }) (Record, error) {
	var nameValue, valueValue, versionValue any
	if err := row.Scan(&nameValue, &valueValue, &versionValue); err != nil {
		return Record{}, err
	}
	name, nameOK := nameValue.(string)
	value, valueOK := valueValue.([]byte)
	encodedVersion, versionOK := versionValue.([]byte)
	if !nameOK || !valueOK || !versionOK {
		return Record{}, ErrInvalidRecord
	}
	version, err := decodeVersion(encodedVersion)
	if err != nil {
		return Record{}, err
	}
	record := Record{Name: name, Version: version, Value: value}
	return record, record.Validate()
}

func writeRecords(tx *sql.Tx, records []Record) (bool, error) {
	// Fixed-width big-endian version bytes preserve schema/revision/digest order.
	stmt, err := tx.Prepare(`INSERT INTO documents(name, value, version) VALUES(?, ?, ?)
ON CONFLICT(name) DO UPDATE SET value=excluded.value, version=excluded.version
WHERE excluded.version > documents.version`)
	if err != nil {
		return false, err
	}
	defer stmt.Close()
	var changed bool
	for _, record := range records {
		value := record.Value
		if value == nil {
			value = []byte{}
		}
		result, err := stmt.Exec(record.Name, value, record.Version.bytes())
		if err != nil {
			return false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		changed = changed || count != 0
	}
	return changed, nil
}
