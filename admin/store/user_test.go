// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateUser_Success(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	user, err := st.CreateUser(context.Background(), "admin", "$2a$10$hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if user.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if user.Username != "admin" {
		t.Errorf("Username = %q, want %q", user.Username, "admin")
	}
}

func TestCreateUser_DuplicateUsername_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.CreateUser(ctx, "admin", "$2a$10$hash1"); err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	_, err := st.CreateUser(ctx, "admin", "$2a$10$hash2")
	if !errors.Is(err, ErrDuplicateUsername) {
		t.Errorf("error = %v, want %v", err, ErrDuplicateUsername)
	}
}

func TestGetUserByID_Exists_ReturnsUser(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	created, err := st.CreateUser(ctx, "admin", "$2a$10$hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	got, err := st.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.Username != "admin" {
		t.Errorf("Username = %q, want %q", got.Username, "admin")
	}
	if got.PasswordHash != "$2a$10$hash" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "$2a$10$hash")
	}
}

func TestGetUserByID_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	_, err := st.GetUserByID(context.Background(), 999)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestGetUserByUsername_Exists_ReturnsUser(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.CreateUser(ctx, "admin", "$2a$10$hash"); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	got, err := st.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if got.Username != "admin" {
		t.Errorf("Username = %q, want %q", got.Username, "admin")
	}
}

func TestGetUserByUsername_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	_, err := st.GetUserByUsername(context.Background(), "nonexistent")
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestUpdateUserPassword_Success(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "admin", "$2a$10$oldhash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	err = st.UpdateUserPassword(ctx, user.ID, "$2a$10$newhash")
	if err != nil {
		t.Fatalf("UpdateUserPassword() error = %v", err)
	}

	got, err := st.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.PasswordHash != "$2a$10$newhash" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "$2a$10$newhash")
	}
}

func TestUpdateUserPassword_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	err := st.UpdateUserPassword(context.Background(), 999, "$2a$10$hash")
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestCreateSession_And_GetByToken(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "admin", "$2a$10$hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	err = st.CreateSession(ctx, user.ID, "tok-abc-123", expiresAt)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	sess, err := st.GetSessionByToken(ctx, "tok-abc-123")
	if err != nil {
		t.Fatalf("GetSessionByToken() error = %v", err)
	}
	if sess.UserID != user.ID {
		t.Errorf("UserID = %d, want %d", sess.UserID, user.ID)
	}
	// Token is stored as a SHA-256 hash, so it won't match the raw value.
	if sess.Token == "" {
		t.Error("expected non-empty token hash")
	}
	if sess.Token == "tok-abc-123" {
		t.Error("token should be stored as a hash, not raw")
	}
}

func TestGetSessionByToken_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	_, err := st.GetSessionByToken(context.Background(), "nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, ErrSessionNotFound)
	}
}

func TestTouchSession_UpdatesLastActiveAt(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	user, _ := st.CreateUser(ctx, "admin", "$2a$10$hash")
	expiresAt := time.Now().Add(24 * time.Hour)
	_ = st.CreateSession(ctx, user.ID, "tok-touch", expiresAt)

	before, _ := st.GetSessionByToken(ctx, "tok-touch")
	if err := st.TouchSession(ctx, "tok-touch"); err != nil {
		t.Fatalf("TouchSession() error = %v", err)
	}
	after, _ := st.GetSessionByToken(ctx, "tok-touch")

	if after.LastActiveAt.Before(before.LastActiveAt) {
		t.Errorf("LastActiveAt should not go backward after touch")
	}
}

func TestDeleteSession_RemovesSession(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	user, _ := st.CreateUser(ctx, "admin", "$2a$10$hash")
	_ = st.CreateSession(ctx, user.ID, "tok-del", time.Now().Add(time.Hour))

	if err := st.DeleteSession(ctx, "tok-del"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	_, err := st.GetSessionByToken(ctx, "tok-del")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("after delete: error = %v, want %v", err, ErrSessionNotFound)
	}
}

func TestDeleteUserSessions_RemovesAll(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	user, _ := st.CreateUser(ctx, "admin", "$2a$10$hash")
	_ = st.CreateSession(ctx, user.ID, "tok-a", time.Now().Add(time.Hour))
	_ = st.CreateSession(ctx, user.ID, "tok-b", time.Now().Add(time.Hour))

	if err := st.DeleteUserSessions(ctx, user.ID); err != nil {
		t.Fatalf("DeleteUserSessions() error = %v", err)
	}

	_, errA := st.GetSessionByToken(ctx, "tok-a")
	_, errB := st.GetSessionByToken(ctx, "tok-b")
	if !errors.Is(errA, ErrSessionNotFound) || !errors.Is(errB, ErrSessionNotFound) {
		t.Errorf("sessions should be deleted; errA = %v, errB = %v", errA, errB)
	}
}

func TestDeleteOtherSessions_KeepsSpecifiedToken(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	user, _ := st.CreateUser(ctx, "admin", "$2a$10$hash")
	_ = st.CreateSession(ctx, user.ID, "tok-keep", time.Now().Add(time.Hour))
	_ = st.CreateSession(ctx, user.ID, "tok-remove-a", time.Now().Add(time.Hour))
	_ = st.CreateSession(ctx, user.ID, "tok-remove-b", time.Now().Add(time.Hour))

	if err := st.DeleteOtherSessions(ctx, user.ID, "tok-keep"); err != nil {
		t.Fatalf("DeleteOtherSessions() error = %v", err)
	}

	// Kept session should still exist.
	if _, err := st.GetSessionByToken(ctx, "tok-keep"); err != nil {
		t.Errorf("kept session should exist: %v", err)
	}

	// Other sessions should be deleted.
	_, errA := st.GetSessionByToken(ctx, "tok-remove-a")
	_, errB := st.GetSessionByToken(ctx, "tok-remove-b")
	if !errors.Is(errA, ErrSessionNotFound) || !errors.Is(errB, ErrSessionNotFound) {
		t.Errorf("other sessions should be deleted; errA = %v, errB = %v", errA, errB)
	}
}

func TestDeleteExpiredSessions_RemovesExpiredOnly(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	user, _ := st.CreateUser(ctx, "admin", "$2a$10$hash")

	// One expired, one active.
	_ = st.CreateSession(ctx, user.ID, "tok-expired", time.Now().Add(-time.Hour))
	_ = st.CreateSession(ctx, user.ID, "tok-active", time.Now().Add(time.Hour))

	n, err := st.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions() error = %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}

	_, err = st.GetSessionByToken(ctx, "tok-active")
	if err != nil {
		t.Errorf("active session should still exist: %v", err)
	}
}
