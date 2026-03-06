// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors for instance operations.
var (
	ErrInstanceNotFound = errors.New("instance not found")
	ErrDuplicateAddress = errors.New("duplicate instance address")
)

// Instance represents a registered proxy instance.
type Instance struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Address    string     `json:"address"`
	Status     string     `json:"status"`
	Version    string     `json:"version"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ListInstances returns all registered instances ordered by name.
func (s *Store) ListInstances(ctx context.Context) ([]Instance, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, address, status, version, last_seen_at, created_at, updated_at
		 FROM instances ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing instances: %w", err)
	}
	defer rows.Close()

	var instances []Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

// GetInstance returns a single instance by ID.
func (s *Store) GetInstance(ctx context.Context, id int64) (*Instance, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, address, status, version, last_seen_at, created_at, updated_at
		 FROM instances WHERE id = ?`, id)

	inst, err := scanInstanceRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting instance %d: %w", id, err)
	}
	return &inst, nil
}

// CreateInstance inserts a new instance and returns it.
func (s *Store) CreateInstance(ctx context.Context, name, address string) (*Instance, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO instances (name, address) VALUES (?, ?)`, name, address)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrDuplicateAddress
		}
		return nil, fmt.Errorf("creating instance: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert ID: %w", err)
	}
	return s.GetInstance(ctx, id)
}

// UpdateInstance updates instance name and address by ID and returns the updated instance.
func (s *Store) UpdateInstance(ctx context.Context, id int64, name, address string) (*Instance, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE instances SET name = ?, address = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		name, address, id)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrDuplicateAddress
		}
		return nil, fmt.Errorf("updating instance %d: %w", id, err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return nil, ErrInstanceNotFound
	}
	return s.GetInstance(ctx, id)
}

// DeleteInstance removes an instance by ID.
func (s *Store) DeleteInstance(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting instance %d: %w", id, err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

// SetInstanceHealthy marks an instance as healthy with the given version.
func (s *Store) SetInstanceHealthy(ctx context.Context, id int64, version string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE instances SET status = 'healthy', version = ?, last_seen_at = CURRENT_TIMESTAMP,
		 updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		version, id)
	if err != nil {
		return fmt.Errorf("setting instance %d healthy: %w", id, err)
	}
	return nil
}

// SetInstanceUnreachable marks an instance as unreachable.
func (s *Store) SetInstanceUnreachable(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE instances SET status = 'unreachable', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		id)
	if err != nil {
		return fmt.Errorf("setting instance %d unreachable: %w", id, err)
	}
	return nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanInstance(s scanner) (Instance, error) {
	var inst Instance
	err := s.Scan(
		&inst.ID, &inst.Name, &inst.Address, &inst.Status,
		&inst.Version, &inst.LastSeenAt, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return Instance{}, fmt.Errorf("scanning instance: %w", err)
	}
	return inst, nil
}

func scanInstanceRow(row *sql.Row) (Instance, error) {
	var inst Instance
	err := row.Scan(
		&inst.ID, &inst.Name, &inst.Address, &inst.Status,
		&inst.Version, &inst.LastSeenAt, &inst.CreatedAt, &inst.UpdatedAt,
	)
	return inst, err
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
