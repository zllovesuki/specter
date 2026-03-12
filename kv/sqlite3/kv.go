package sqlite3

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go.miragespace.co/specter/spec/chord"

	"go.uber.org/zap"
)

type SimpleEntry struct {
	Key   []byte
	Value []byte
}

type PrefixEntry struct {
	Prefix []byte
	Child  []byte
}

type LeaseEntry struct {
	Owner []byte
	Token uint64
}

type Config struct {
	Logger  *zap.Logger
	HashFn  chord.HashFn
	DataDir string
}

type SqliteKV struct {
	logger *zap.Logger
	hashFn chord.HashFn
	reader *sql.DB
	writer *sql.DB
	stmts  *statements
}

func (c Config) validate() error {
	if c.Logger == nil {
		return fmt.Errorf("nil Logger is invalid")
	}
	if c.HashFn == nil {
		return fmt.Errorf("nil HashFn is invalid")
	}
	if c.DataDir == "" {
		return fmt.Errorf("empty DataDir is invalid")
	}
	return nil
}

func New(cfg Config) (*SqliteKV, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	dbDir := filepath.Join(cfg.DataDir, "sqlite3")
	if err := os.MkdirAll(dbDir, 0750); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dbDir, "db")

	var (
		reader *sql.DB
		writer *sql.DB
		stmts  *statements
		err    error
	)

	defer func() {
		if err == nil {
			return
		}
		if stmts != nil {
			stmts.close()
		}
		if reader != nil {
			_ = reader.Close()
		}
		if writer != nil {
			_ = writer.Close()
		}
	}()

	reader, err = openSQLite(cfg.Logger, dbPath)
	if err != nil {
		return nil, err
	}
	writer, err = openSQLite(cfg.Logger, dbPath)
	if err != nil {
		return nil, err
	}

	reader.SetMaxOpenConns(max(4, runtime.NumCPU()))
	// prevent SQLITE_BUSY
	writer.SetMaxOpenConns(1)

	if err := migrate(writer); err != nil {
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	stmts, err = prepareStatements(reader, writer)
	if err != nil {
		return nil, fmt.Errorf("preparing statements: %w", err)
	}

	return &SqliteKV{
		logger: cfg.Logger,
		hashFn: cfg.HashFn,
		reader: reader,
		writer: writer,
		stmts:  stmts,
	}, nil
}

func (s *SqliteKV) Close() {
	if s == nil {
		return
	}
	if s.stmts != nil {
		s.stmts.close()
	}
	if s.reader != nil {
		_ = s.reader.Close()
	}
	if s.writer != nil {
		_ = s.writer.Close()
	}
}

var _ chord.KVProvider = (*SqliteKV)(nil)
