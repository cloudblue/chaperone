// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register sqlite driver
)

// Store wraps a SQLite database connection for the admin portal.
type Store struct {
	db *sql.DB
}

// DB returns the underlying database connection for use by API handlers.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Open creates a new Store with the given SQLite database path.
// It configures WAL mode, busy timeout, and foreign keys, then runs
// any pending schema migrations.
func Open(ctx context.Context, dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close() // best-effort cleanup; primary error is the pragma failure
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // best-effort cleanup; primary error is the ping failure
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close() // best-effort cleanup; primary error is the migration failure
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}
	return nil
}
