package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies every *.up.sql file under migrations/ that has not yet
// been recorded in schema_migrations, in filename order. It is
// intentionally dependency-free rather than pulling in a full migration
// framework for a handful of statements.
func Migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("migrate: bootstrap failed: %w", err)
	}

	entries, err := fs.Glob(migrationFiles, "migrations/*.up.sql")
	if err != nil {
		return fmt.Errorf("migrate: glob failed: %w", err)
	}
	sort.Strings(entries)

	for _, path := range entries {
		version := path
		var exists bool
		if err := conn.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("migrate: check %s failed: %w", version, err)
		}
		if exists {
			continue
		}

		sqlBytes, err := migrationFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("migrate: read %s failed: %w", version, err)
		}

		tx, err := conn.Begin()
		if err != nil {
			return fmt.Errorf("migrate: begin tx for %s failed: %w", version, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate: apply %s failed: %w", version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate: record %s failed: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate: commit %s failed: %w", version, err)
		}
	}

	return nil
}
