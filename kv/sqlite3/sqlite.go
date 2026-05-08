package sqlite3

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"fmt"

	ncrsqlite3 "github.com/ncruces/go-sqlite3"
	ncrdriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/vfs"
	"go.uber.org/zap"
)

const sqliteMaxMemory = 64 * 1024 * 1024

type memoryLimitConnector struct {
	sqldriver.Connector
	max int64
}

func (c memoryLimitConnector) Connect(ctx context.Context) (sqldriver.Conn, error) {
	return c.Connector.Connect(ncrsqlite3.WithMaxMemory(ctx, c.max))
}

func Initialize(cacheDir string) error {
	_ = cacheDir
	return nil
}

func openSQLite(logger *zap.Logger, dbPath string) (*sql.DB, error) {
	logger.Info("SQLite via wasm2go",
		zap.Int("maxMemory", sqliteMaxMemory),
		zap.Bool("lock", vfs.SupportsFileLocking),
		zap.Bool("shm", vfs.SupportsSharedMemory),
	)

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(1)&_txlock=immediate", dbPath)
	connector, err := (&ncrdriver.SQLite{}).OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(memoryLimitConnector{
		Connector: connector,
		max:       sqliteMaxMemory,
	}), nil
}
