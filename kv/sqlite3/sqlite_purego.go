//go:build !illumos

package sqlite3

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSQLite(dbpath string) (gorm.Dialector, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(1)&_pragma=page_size(4096)&_txlock=immediate", dbpath)
	db := sqlite.Open(dsn)
	return db, nil
}
