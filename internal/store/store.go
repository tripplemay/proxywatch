package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed migrations.sql
var migrationsSQL string

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := db.Exec(migrationsSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := ensureColumns(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// ensureColumns adds new columns to existing tables for backward-compat with
// older DB files that pre-date a column. Uses PRAGMA to detect existence,
// then ALTER TABLE if missing.
func ensureColumns(db *sql.DB) error {
	cols := []struct{ table, col, ddl string }{
		{"notifications", "buttons", "TEXT"},
	}
	for _, c := range cols {
		var n int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
			c.table, c.col,
		).Scan(&n)
		if err != nil {
			return fmt.Errorf("pragma_table_info(%s): %w", c.table, err)
		}
		if n > 0 {
			continue // already exists
		}
		_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, c.col, c.ddl))
		if err != nil {
			return fmt.Errorf("add column %s.%s: %w", c.table, c.col, err)
		}
	}
	return nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Close() error { return s.db.Close() }
