package sqlite3

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestValidateMigrationSequence(t *testing.T) {
	as := require.New(t)

	err := validateMigrationSequence([]migration{
		{version: 1, name: "0001-initial-schema.sql"},
		{version: 3, name: "0003-add-thing.sql"},
	})
	as.Error(err)
	as.ErrorContains(err, "expected migration version 0002")
}

func TestMigrateRejectsPartialLegacySchema(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db")

	db, err := openSQLite(logger, dbPath)
	as.NoError(err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	_, err = db.Exec("CREATE TABLE `simple_entries` (`key` blob,`value` blob,PRIMARY KEY (`key`))")
	as.NoError(err)

	err = migrate(db)
	as.Error(err)
	as.ErrorContains(err, "partial sqlite schema")
}

func TestMigrateFreshDatabase(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db")

	db, err := openSQLite(logger, dbPath)
	as.NoError(err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrations, err := loadMigrations()
	as.NoError(err)
	as.NotEmpty(migrations)

	err = migrate(db)
	as.NoError(err)

	uv, err := getUserVersion(db)
	as.NoError(err)
	as.Equal(migrations[len(migrations)-1].version, uv)

	ok, err := schemaLooksLikeV1(db)
	as.NoError(err)
	as.True(ok)
}

func TestMigrateAdoptsLegacySchema(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db")

	db, err := openSQLite(logger, dbPath)
	as.NoError(err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrations, err := loadMigrations()
	as.NoError(err)
	as.NotEmpty(migrations)

	_, err = db.Exec(migrations[0].sql)
	as.NoError(err)

	uv, err := getUserVersion(db)
	as.NoError(err)
	as.Zero(uv)

	err = migrate(db)
	as.NoError(err)

	uv, err = getUserVersion(db)
	as.NoError(err)
	as.Equal(migrations[len(migrations)-1].version, uv)
}

func TestMigrateRejectsNewerUserVersion(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db")

	db, err := openSQLite(logger, dbPath)
	as.NoError(err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrations, err := loadMigrations()
	as.NoError(err)
	as.NotEmpty(migrations)

	err = setUserVersion(db, migrations[len(migrations)-1].version+1)
	as.NoError(err)

	err = migrate(db)
	as.Error(err)
	as.ErrorContains(err, "newer than supported")
}
