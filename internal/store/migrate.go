package store

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

const migrationsTable = `
CREATE TABLE IF NOT EXISTS _migrations (
  name       TEXT PRIMARY KEY,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`

// Migrate applies every *.sql file in src in lexical order, skipping any whose
// filename is already recorded in the _migrations tracking table. Safe to run
// at every startup.
func Migrate(db *sql.DB, src fs.FS) error {
	if _, err := db.Exec(migrationsTable); err != nil {
		return fmt.Errorf("create _migrations: %w", err)
	}

	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	applied, err := loadApplied(db)
	if err != nil {
		return err
	}

	for _, name := range files {
		if applied[name] {
			continue
		}
		body, err := fs.ReadFile(src, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO _migrations(name) VALUES (?)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
	}
	return nil
}

func loadApplied(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM _migrations`)
	if err != nil {
		return nil, fmt.Errorf("query _migrations: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}
