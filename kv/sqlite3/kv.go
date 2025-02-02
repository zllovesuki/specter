package sqlite3

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"go.miragespace.co/specter/spec/chord"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

type SimpleEntry struct {
	Key   []byte `gorm:"primaryKey"`
	Value []byte
}

type PrefixEntry struct {
	Prefix []byte `gorm:"primaryKey"`
	Child  []byte `gorm:"primaryKey"`
}

type LeaseEntry struct {
	Owner []byte `gorm:"primaryKey"`
	Token uint64
}

type Config struct {
	Logger  *zap.Logger
	HasnFn  chord.HashFn
	DataDir string
}

type SqliteKV struct {
	logger *zap.Logger
	hashFn chord.HashFn
	reader *gorm.DB
	writer *gorm.DB
}

func (c Config) validate() error {
	if c.Logger == nil {
		return fmt.Errorf("nil Logger is invalid")
	}
	if c.HasnFn == nil {
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

	readDb, err := openSQLite(cfg.Logger, dbPath)
	if err != nil {
		return nil, err
	}
	writeDb, err := openSQLite(cfg.Logger, dbPath)
	if err != nil {
		return nil, err
	}

	logger := zapgorm2.New(cfg.Logger)
	logger.IgnoreRecordNotFoundError = true
	logger.SlowThreshold = time.Millisecond * 500

	reader, err := gorm.Open(readDb, &gorm.Config{
		Logger:         logger,
		PrepareStmt:    true,
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}

	writer, err := gorm.Open(writeDb, &gorm.Config{
		Logger:         logger,
		PrepareStmt:    true,
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}

	readerDb, err := reader.DB()
	if err != nil {
		return nil, err
	}
	readerDb.SetMaxOpenConns(max(4, runtime.NumCPU()))
	// prevent SQLITE_BUSY
	writerDb, err := writer.DB()
	if err != nil {
		return nil, err
	}
	writerDb.SetMaxOpenConns(1)

	if err := writer.AutoMigrate(&KeyTracker{}); err != nil {
		return nil, err
	}
	if err := writer.AutoMigrate(&SimpleEntry{}, &PrefixEntry{}, &LeaseEntry{}); err != nil {
		return nil, err
	}

	return &SqliteKV{
		logger: cfg.Logger,
		hashFn: cfg.HasnFn,
		reader: reader,
		writer: writer,
	}, nil
}

var _ chord.KVProvider = (*SqliteKV)(nil)
