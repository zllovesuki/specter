// Package sqlite opens the SQLite driver with Specter's per-connection memory limit.
package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

	"github.com/ncruces/go-sqlite3"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

const MaxMemory = 64 << 20

// Open applies the memory limit and optional setup to every physical connection,
// including replacements opened by database/sql's background goroutine.
func Open(dsn string, setup func(*sqlite3.Conn) error) (*sql.DB, error) {
	c, err := (&sqlitedriver.SQLite{}).OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector{Connector: c, setup: setup}), nil
}

type connector struct {
	driver.Connector
	setup func(*sqlite3.Conn) error
}

func (c connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(sqlite3.WithMaxMemory(ctx, MaxMemory))
	if err != nil {
		return nil, err
	}
	if c.setup != nil {
		raw := conn.(sqlitedriver.Conn).Raw()
		previous := raw.SetInterrupt(ctx)
		err = c.setup(raw)
		raw.SetInterrupt(previous)
		if err != nil {
			return nil, errors.Join(err, conn.Close())
		}
	}
	return conn, nil
}
