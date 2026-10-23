package sqlite3

import (
	"database/sql"
	"fmt"

	"go.miragespace.co/specter/util/sqlite"

	"github.com/ncruces/go-sqlite3/vfs"
	"go.uber.org/zap"
)

func Initialize(cacheDir string) error {
	_ = cacheDir
	return nil
}

func openSQLite(logger *zap.Logger, dbPath string) (*sql.DB, error) {
	logger.Info("SQLite via wasm2go",
		zap.Int("maxMemory", sqlite.MaxMemory),
		zap.Bool("lock", vfs.SupportsFileLocking),
		zap.Bool("shm", vfs.SupportsSharedMemory),
	)

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(1)&_txlock=immediate", dbPath)
	return sqlite.Open(dsn, nil)
}
