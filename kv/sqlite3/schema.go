package sqlite3

import (
	"database/sql"
	"fmt"
)

// Frozen schema v1: exact DDL captured from GORM AutoMigrate output.
// These must match the tables created by the previous GORM-based implementation.
const schemaVersion = 1

func getUserVersion(db *sql.DB) (int, error) {
	var v int
	err := db.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}

func setUserVersion(db *sql.DB, v int) error {
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
	return err
}

func setTxUserVersion(tx *sql.Tx, v int) error {
	_, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
	return err
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name=?", name,
	).Scan(&count)
	return count > 0, err
}

func indexExists(db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_schema WHERE type='index' AND name=?", name,
	).Scan(&count)
	return count > 0, err
}

func schemaHasAnyV1Objects(db *sql.DB) (bool, error) {
	for _, tbl := range []string{"key_trackers", "simple_entries", "prefix_entries", "lease_entries"} {
		ok, err := tableExists(db, tbl)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	ok, err := indexExists(db, "idx_hash")
	if err != nil {
		return false, err
	}
	return ok, nil
}

// schemaLooksLikeV1 checks whether the expected v1 tables exist.
func schemaLooksLikeV1(db *sql.DB) (bool, error) {
	for _, tbl := range []string{"key_trackers", "simple_entries", "prefix_entries", "lease_entries"} {
		ok, err := tableExists(db, tbl)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	ok, err := indexExists(db, "idx_hash")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return true, nil
}

func applyMigration(db *sql.DB, migration migration) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(migration.sql); err != nil {
		return err
	}
	if err = setTxUserVersion(tx, migration.version); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

// migrate brings the database schema up to the current version.
// It handles three cases:
//   - Fresh DB (no tables, user_version=0): create v1 schema and set user_version=1.
//   - Legacy GORM DB (v1 tables exist, user_version=0): accept as v1 baseline, set user_version=1.
//   - Already migrated DB (user_version=1): no-op.
func migrate(db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}
	latestVersion := 0
	if len(migrations) > 0 {
		latestVersion = migrations[len(migrations)-1].version
	}

	uv, err := getUserVersion(db)
	if err != nil {
		return fmt.Errorf("reading user_version: %w", err)
	}

	if uv == latestVersion {
		return nil
	}

	if uv > latestVersion {
		return fmt.Errorf("database user_version %d is newer than supported version %d", uv, latestVersion)
	}

	// uv == 0: either fresh or legacy GORM DB
	if uv == 0 {
		isLegacy, err := schemaLooksLikeV1(db)
		if err != nil {
			return fmt.Errorf("inspecting schema: %w", err)
		}

		if isLegacy {
			if err := setUserVersion(db, schemaVersion); err != nil {
				return err
			}
			uv = schemaVersion
		} else {
			hasObjects, err := schemaHasAnyV1Objects(db)
			if err != nil {
				return fmt.Errorf("inspecting partial schema: %w", err)
			}
			if hasObjects {
				return fmt.Errorf("database user_version %d has unexpected partial sqlite schema", uv)
			}
		}
	}

	for _, migration := range migrations {
		if migration.version <= uv {
			continue
		}
		if err := applyMigration(db, migration); err != nil {
			return fmt.Errorf("applying migration %s: %w", migration.name, err)
		}
	}

	return nil
}
