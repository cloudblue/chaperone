// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

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
//
// Pragmas are passed via the DSN so that every connection in the
// database/sql pool receives them, not just the first one.
func Open(ctx context.Context, dbPath string) (*Store, error) {
	dsn := buildDSN(dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
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

// buildDSN constructs a SQLite DSN with per-connection pragmas.
// Using _pragma query parameters ensures every pooled connection
// gets WAL mode, a busy timeout, and foreign key enforcement.
func buildDSN(dbPath string) string {
	v := url.Values{}
	v.Add("_pragma", "journal_mode(WAL)")
	v.Add("_pragma", "busy_timeout(5000)")
	v.Add("_pragma", "foreign_keys(1)")
	return dbPath + "?" + v.Encode()
}

// Close closes the database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}
	return nil
}
