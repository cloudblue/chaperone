// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"log/slog"
)

type migration struct {
	Version     int
	Description string
	SQL         string
}

var migrations = []migration{
	{
		Version:     1,
		Description: "initial schema",
		SQL: `
CREATE TABLE instances (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	address TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL DEFAULT 'unknown',
	version TEXT NOT NULL DEFAULT '',
	last_seen_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token TEXT NOT NULL UNIQUE,
	expires_at TIMESTAMP NOT NULL,
	last_active_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	action TEXT NOT NULL,
	instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL,
	detail TEXT,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);
CREATE INDEX idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_sessions_token ON sessions(token);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
`,
	},
	{
		Version:     2,
		Description: "add FTS5 index for audit log full-text search",
		SQL: `
CREATE VIRTUAL TABLE audit_log_fts USING fts5(
	detail,
	content='audit_log',
	content_rowid='id'
);

CREATE TRIGGER audit_log_ai AFTER INSERT ON audit_log BEGIN
	INSERT INTO audit_log_fts(rowid, detail) VALUES (new.id, new.detail);
END;

CREATE TRIGGER audit_log_ad AFTER DELETE ON audit_log BEGIN
	INSERT INTO audit_log_fts(audit_log_fts, rowid, detail) VALUES('delete', old.id, old.detail);
END;

CREATE TRIGGER audit_log_au AFTER UPDATE ON audit_log BEGIN
	INSERT INTO audit_log_fts(audit_log_fts, rowid, detail) VALUES('delete', old.id, old.detail);
	INSERT INTO audit_log_fts(rowid, detail) VALUES (new.id, new.detail);
END;
`,
	},
}

func (s *Store) migrate(ctx context.Context) error {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	current, err := s.currentSchemaVersion(ctx)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}

		slog.Info("applying migration",
			"version", m.Version,
			"description", m.Description,
		)

		if err := s.applyMigration(ctx, m); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) ensureMigrationsTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}
	return nil
}

func (s *Store) currentSchemaVersion(ctx context.Context) (int, error) {
	var current int
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current)
	if err != nil {
		return 0, fmt.Errorf("reading current schema version: %w", err)
	}
	return current, nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction for migration %d: %w", m.Version, err)
	}

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.Error("rolling back migration", "version", m.Version, "error", rbErr)
		}
		return fmt.Errorf("applying migration %d (%s): %w", m.Version, m.Description, err)
	}

	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.Error("rolling back migration", "version", m.Version, "error", rbErr)
		}
		return fmt.Errorf("recording migration %d: %w", m.Version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %d: %w", m.Version, err)
	}

	return nil
}
