package sqlite3

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		version, err := parseMigrationVersion(name)
		if err != nil {
			return nil, err
		}

		body, err := fs.ReadFile(migrationFiles, path.Join("migrations", name))
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, migration{
			version: version,
			name:    name,
			sql:     string(body),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	if err := validateMigrationSequence(migrations); err != nil {
		return nil, err
	}

	return migrations, nil
}

func validateMigrationSequence(migrations []migration) error {
	for i, migration := range migrations {
		expected := i + 1
		if migration.version != expected {
			return fmt.Errorf("expected migration version %04d, got %04d (%s)", expected, migration.version, migration.name)
		}
	}
	return nil
}

func parseMigrationVersion(name string) (int, error) {
	base := path.Base(name)
	if !strings.HasSuffix(base, ".sql") {
		return 0, fmt.Errorf("invalid migration name %q", name)
	}

	trimmed := strings.TrimSuffix(base, ".sql")
	prefix, _, ok := strings.Cut(trimmed, "-")
	if !ok || prefix == "" {
		return 0, fmt.Errorf("invalid migration name %q", name)
	}

	version, err := strconv.Atoi(prefix)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("invalid migration version in %q", name)
	}
	return version, nil
}
