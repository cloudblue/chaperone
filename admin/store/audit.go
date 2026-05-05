// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Username   string    `json:"user"`
	Action     string    `json:"action"`
	InstanceID *int64    `json:"instance_id,omitempty"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditFilter specifies query parameters for listing audit entries.
type AuditFilter struct {
	UserID     *int64
	Action     string
	InstanceID *int64
	From       *time.Time
	To         *time.Time
	Query      string // full-text search on detail
	Page       int
	PerPage    int
}

// AuditPage is a paginated response of audit entries.
type AuditPage struct {
	Items []AuditEntry `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
}

// InsertAuditEntry records an action in the audit log.
func (s *Store) InsertAuditEntry(ctx context.Context, userID int64, action string, instanceID *int64, detail string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (user_id, action, instance_id, detail) VALUES (?, ?, ?, ?)`,
		userID, action, instanceID, detail)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}
	return nil
}

// ListAuditEntries returns a paginated, filtered list of audit entries.
func (s *Store) ListAuditEntries(ctx context.Context, filter AuditFilter) (*AuditPage, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 {
		filter.PerPage = 20
	}

	conditions, args := buildAuditConditions(filter)
	joins := "JOIN users u ON a.user_id = u.id"
	if filter.Query != "" {
		joins += " JOIN audit_log_fts f ON a.id = f.rowid"
	}

	where := "1=1"
	if len(conditions) > 0 {
		where = strings.Join(conditions, " AND ")
	}

	// Count total matching entries.
	// Dynamic SQL is safe: joins and where are built from fixed strings, not user input.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log a %s WHERE %s", joins, where) //nolint:gosec // G201 -- see above
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting audit entries: %w", err)
	}

	// Fetch the page.
	offset := (filter.Page - 1) * filter.PerPage
	dataQuery := fmt.Sprintf( //nolint:gosec // G201 -- joins/where built from fixed strings
		`SELECT a.id, a.user_id, u.username, a.action, a.instance_id, a.detail, a.created_at
		 FROM audit_log a %s
		 WHERE %s
		 ORDER BY a.created_at DESC
		 LIMIT ? OFFSET ?`, joins, where)
	dataArgs := append(args, filter.PerPage, offset) //nolint:gocritic // append to copy is intentional

	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("listing audit entries: %w", err)
	}
	defer rows.Close()

	items := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Action, &e.InstanceID, &e.Detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit entries: %w", err)
	}

	return &AuditPage{Items: items, Total: total, Page: filter.Page}, nil
}

// DeleteAuditEntriesBefore removes audit entries older than the given time.
// Returns the number of deleted rows.
func (s *Store) DeleteAuditEntriesBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM audit_log WHERE created_at < ?`, before)
	if err != nil {
		return 0, fmt.Errorf("deleting old audit entries: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking rows affected: %w", err)
	}
	return n, nil
}

func buildAuditConditions(filter AuditFilter) (conditions []string, args []any) {
	if filter.UserID != nil {
		conditions = append(conditions, "a.user_id = ?")
		args = append(args, *filter.UserID)
	}
	if filter.Action != "" {
		conditions = append(conditions, "a.action = ?")
		args = append(args, filter.Action)
	}
	if filter.InstanceID != nil {
		conditions = append(conditions, "a.instance_id = ?")
		args = append(args, *filter.InstanceID)
	}
	if filter.From != nil {
		conditions = append(conditions, "a.created_at >= ?")
		args = append(args, *filter.From)
	}
	if filter.To != nil {
		conditions = append(conditions, "a.created_at <= ?")
		args = append(args, *filter.To)
	}
	if filter.Query != "" {
		conditions = append(conditions, "audit_log_fts MATCH ?")
		args = append(args, ftsQuote(filter.Query))
	}

	return conditions, args
}

// ftsQuote wraps a user query in double quotes so FTS5 treats it as a
// literal phrase. Internal double quotes are escaped per FTS5 rules.
func ftsQuote(q string) string {
	escaped := strings.ReplaceAll(q, `"`, `""`)
	return `"` + escaped + `"`
}
