// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for user and session operations.
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrDuplicateUsername = errors.New("duplicate username")
	ErrSessionNotFound   = errors.New("session not found")
)

// User represents a portal admin user.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Session represents an active user session.
type Session struct {
	ID           int64
	UserID       int64
	TokenHash    string
	ExpiresAt    time.Time
	LastActiveAt time.Time
	CreatedAt    time.Time
}

// CreateUser inserts a new user with the given bcrypt password hash.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (*User, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, passwordHash)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrDuplicateUsername
		}
		return nil, fmt.Errorf("creating user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert ID: %w", err)
	}
	return s.GetUserByID(ctx, id)
}

// GetUserByID returns a user by their ID.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at, updated_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting user %d: %w", id, err)
	}
	return &u, nil
}

// GetUserByUsername returns a user by their username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at, updated_at FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting user %q: %w", username, err)
	}
	return &u, nil
}

// UpdateUserPassword changes a user's password hash.
func (s *Store) UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		passwordHash, userID)
	if err != nil {
		return fmt.Errorf("updating password for user %d: %w", userID, err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// CreateSession inserts a new session record.
// The raw token is hashed before storage; callers always pass raw tokens.
func (s *Store) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)`,
		userID, hashToken(token), expiresAt)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

// GetSessionByToken looks up a session by its raw token (hashed for lookup).
func (s *Store) GetSessionByToken(ctx context.Context, token string) (*Session, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, token, expires_at, last_active_at, created_at
		 FROM sessions WHERE token = ?`, hashToken(token)).
		Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.ExpiresAt, &sess.LastActiveAt, &sess.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	return &sess, nil
}

// TouchSession updates the last_active_at timestamp for idle timeout tracking.
// Accepts the raw token (hashed for lookup).
func (s *Store) TouchSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_active_at = CURRENT_TIMESTAMP WHERE token = ?`, hashToken(token))
	if err != nil {
		return fmt.Errorf("touching session: %w", err)
	}
	return nil
}

// DeleteSession removes a session by raw token (hashed for lookup).
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE token = ?`, hashToken(token))
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// DeleteUserSessions removes all sessions for a user (password reset).
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting sessions for user %d: %w", userID, err)
	}
	return nil
}

// DeleteOtherSessions removes all sessions for a user except the given token.
// Accepts the raw keepToken (hashed for comparison).
func (s *Store) DeleteOtherSessions(ctx context.Context, userID int64, keepToken string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND token != ?`, userID, hashToken(keepToken))
	if err != nil {
		return fmt.Errorf("deleting other sessions for user %d: %w", userID, err)
	}
	return nil
}

// DeleteExpiredSessions removes sessions past their absolute expiry.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking rows affected: %w", err)
	}
	return n, nil
}

// hashToken computes the SHA-256 hash of a raw session token.
// The database stores hashes so a DB compromise does not leak usable tokens.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
