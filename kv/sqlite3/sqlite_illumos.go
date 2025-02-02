//go:build illumos

package sqlite3

import (
	"errors"

	"gorm.io/gorm"
)

// FIXME: Introduce support for SQLite3 via mattn/go-sqlite3
func openSQLite(_ string) (gorm.Dialector, error) {
	return nil, errors.ErrUnsupported
}
