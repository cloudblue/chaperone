// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
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

	expected := []string{"audit_log", "audit_log_fts", "audit_log_fts_config", "audit_log_fts_data", "audit_log_fts_docsize", "audit_log_fts_idx", "instances", "schema_migrations", "sessions", "users"}
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
	if count != len(migrations) {
		t.Errorf("migration count = %d, want %d", count, len(migrations))
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
	if version != len(migrations) {
		t.Errorf("schema version = %d, want %d", version, len(migrations))
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

func TestOpen_PragmasApplyToAllPoolConnections(t *testing.T) {
	t.Parallel()

	// Arrange — open store and force multiple connections in the pool.
	st := openTestStore(t)
	db := st.DB()
	db.SetMaxOpenConns(4)

	ctx := context.Background()

	// Act — grab several raw connections and check pragmas on each.
	for i := range 4 {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}

		var timeout int
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatalf("conn %d: querying busy_timeout: %v", i, err)
		}
		if timeout != 5000 {
			t.Errorf("conn %d: busy_timeout = %d, want 5000", i, timeout)
		}

		var fk int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatalf("conn %d: querying foreign_keys: %v", i, err)
		}
		if fk != 1 {
			t.Errorf("conn %d: foreign_keys = %d, want 1", i, fk)
		}

		conn.Close()
	}
}

func TestBuildDSN_ContainsPragmas(t *testing.T) {
	t.Parallel()

	dsn := buildDSN("/tmp/test.db")

	// url.Values.Encode() percent-encodes parentheses, so check
	// the encoded form that the driver actually receives.
	for _, want := range []string{
		"_pragma=journal_mode%28WAL%29",
		"_pragma=busy_timeout%285000%29",
		"_pragma=foreign_keys%281%29",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN %q missing pragma %q", dsn, want)
		}
	}

	if !strings.HasPrefix(dsn, "/tmp/test.db?") {
		t.Errorf("DSN %q does not start with expected path", dsn)
	}
}
