// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open(%q) failed: %v", dbPath, err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestOpen_CreatesAllTables(t *testing.T) {
	t.Parallel()

	// Arrange & Act
	st := openTestStore(t)

	// Assert — query sqlite_master for expected tables
	rows, err := st.DB().QueryContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`,
	)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating rows: %v", err)
	}

	expected := []string{"audit_log", "instances", "schema_migrations", "sessions", "users"}
	sort.Strings(tables)

	if len(tables) != len(expected) {
		t.Fatalf("tables = %v, want %v", tables, expected)
	}
	for i, name := range tables {
		if name != expected[i] {
			t.Errorf("table[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestOpen_MigrationIdempotent(t *testing.T) {
	t.Parallel()

	// Arrange — open twice on same DB to verify re-run is safe
	dbPath := filepath.Join(t.TempDir(), "test.db")

	st1, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	st1.Close()

	// Act — open again (should re-run migrate without error)
	st2, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer st2.Close()

	// Assert — schema_migrations still has exactly one entry
	var count int
	if err := st2.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("counting migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("migration count = %d, want 1", count)
	}
}

func TestOpen_WALMode_Enabled(t *testing.T) {
	t.Parallel()

	// Arrange & Act
	st := openTestStore(t)

	// Assert
	var mode string
	if err := st.DB().QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestOpen_SchemaMigrations_TracksVersion(t *testing.T) {
	t.Parallel()

	// Arrange & Act
	st := openTestStore(t)

	// Assert
	var version int
	if err := st.DB().QueryRowContext(context.Background(), "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("querying schema version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestOpen_InvalidPath_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange — directory that doesn't exist
	dbPath := "/nonexistent/dir/test.db"

	// Act
	_, err := Open(context.Background(), dbPath)

	// Assert
	if err == nil {
		t.Error("expected error, got nil")
	}
}
