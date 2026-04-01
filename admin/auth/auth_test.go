// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/admin/store"
)

const testPassword = "securepassword12"

func newTestService(t *testing.T) *Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open(%q) failed: %v", dbPath, err)
	}
	t.Cleanup(func() { st.Close() })
	return NewService(st, 24*time.Hour, 2*time.Hour)
}

func createTestUser(t *testing.T, svc *Service) {
	t.Helper()
	if err := svc.CreateUser(context.Background(), "admin", testPassword); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
}

func loginTestUser(t *testing.T, svc *Service) string {
	t.Helper()
	result, err := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	return result.SessionToken
}

// --- CreateUser ---

func TestCreateUser_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	err := svc.CreateUser(context.Background(), "admin", testPassword)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
}

func TestCreateUser_TooShort_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	err := svc.CreateUser(context.Background(), "admin", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("error = %v, want %v", err, ErrPasswordTooShort)
	}
}

func TestCreateUser_TooLong_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	longPass := strings.Repeat("a", MaxPasswordLength+1)
	err := svc.CreateUser(context.Background(), "admin", longPass)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("error = %v, want %v", err, ErrPasswordTooLong)
	}
}

func TestCreateUser_EmptyUsername_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	err := svc.CreateUser(context.Background(), "", testPassword)
	if !errors.Is(err, ErrInvalidUsername) {
		t.Errorf("error = %v, want %v", err, ErrInvalidUsername)
	}
}

func TestCreateUser_UsernameTooLong_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	longName := strings.Repeat("a", MaxUsernameLength+1)
	err := svc.CreateUser(context.Background(), longName, testPassword)
	if !errors.Is(err, ErrInvalidUsername) {
		t.Errorf("error = %v, want %v", err, ErrInvalidUsername)
	}
}

func TestCreateUser_ControlCharsInUsername_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	err := svc.CreateUser(context.Background(), "admin\x00", testPassword)
	if !errors.Is(err, ErrInvalidUsername) {
		t.Errorf("error = %v, want %v", err, ErrInvalidUsername)
	}
}

func TestCreateUser_Duplicate_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.CreateUser(ctx, "admin", testPassword); err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	err := svc.CreateUser(ctx, "admin", testPassword)
	if !errors.Is(err, store.ErrDuplicateUsername) {
		t.Errorf("error = %v, want %v", err, store.ErrDuplicateUsername)
	}
}

// --- Login ---

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)

	result, err := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.SessionToken == "" {
		t.Error("expected non-empty session token")
	}
	if result.User.Username != "admin" {
		t.Errorf("Username = %q, want %q", result.User.Username, "admin")
	}
}

func TestLogin_WrongPassword_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)

	_, err := svc.Login(context.Background(), "127.0.0.1", "admin", "wrongpassword1")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestLogin_UserNotFound_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	_, err := svc.Login(context.Background(), "127.0.0.1", "nobody", testPassword)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestLogin_RateLimited_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)
	ctx := context.Background()

	for range 5 {
		svc.Login(ctx, "10.0.0.1", "admin", "badpassword00")
	}

	_, err := svc.Login(ctx, "10.0.0.1", "admin", testPassword)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("error = %v, want %v", err, ErrRateLimited)
	}
}

func TestLogin_RateLimit_ResetsOnSuccess(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)
	ctx := context.Background()

	// 4 failures (under limit of 5).
	for range 4 {
		svc.Login(ctx, "10.0.0.2", "admin", "badpassword00")
	}

	// Successful login resets counter.
	if _, err := svc.Login(ctx, "10.0.0.2", "admin", testPassword); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	// 4 more failures should be allowed (counter was reset).
	for range 4 {
		svc.Login(ctx, "10.0.0.2", "admin", "badpassword00")
	}

	// 5th failure should still be under limit.
	_, err := svc.Login(ctx, "10.0.0.2", "admin", "badpassword00")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("error = %v, want %v (should still be under limit)", err, ErrInvalidCredentials)
	}
}

// --- Authenticate ---

func TestAuthenticate_ValidSession_ReturnsUser(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)
	token := loginTestUser(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})

	user, err := svc.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("Username = %q, want %q", user.Username, "admin")
	}
}

func TestAuthenticate_NoCookie_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)

	_, err := svc.Authenticate(req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("error = %v, want %v", err, ErrUnauthenticated)
	}
}

func TestAuthenticate_InvalidToken_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "bad-token"})

	_, err := svc.Authenticate(req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("error = %v, want %v", err, ErrUnauthenticated)
	}
}

func TestAuthenticate_ExpiredSession_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	// Use very short maxAge so session expires immediately.
	svc.maxAge = time.Millisecond
	createTestUser(t, svc)
	token := loginTestUser(t, svc)

	time.Sleep(5 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})

	_, err := svc.Authenticate(req)
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("error = %v, want %v", err, ErrSessionExpired)
	}
}

func TestAuthenticate_IdleSession_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	// Use very short idle timeout.
	svc.idleTimeout = time.Millisecond
	createTestUser(t, svc)
	token := loginTestUser(t, svc)

	time.Sleep(5 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})

	_, err := svc.Authenticate(req)
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("error = %v, want %v", err, ErrSessionExpired)
	}
}

// --- Logout ---

func TestLogout_DeletesSession(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)
	token := loginTestUser(t, svc)

	if err := svc.Logout(context.Background(), token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})

	_, err := svc.Authenticate(req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("after logout: error = %v, want %v", err, ErrUnauthenticated)
	}
}

// --- ChangePassword ---

func TestChangePassword_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)
	ctx := context.Background()

	result, _ := svc.Login(ctx, "127.0.0.1", "admin", testPassword)

	newPass := "newpassword1234"
	if err := svc.ChangePassword(ctx, result.User.ID, result.SessionToken, testPassword, newPass); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	// Old password should fail.
	_, err := svc.Login(ctx, "127.0.0.1", "admin", testPassword)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("old password: error = %v, want %v", err, ErrInvalidCredentials)
	}

	// New password should work.
	if _, err := svc.Login(ctx, "127.0.0.1", "admin", newPass); err != nil {
		t.Errorf("new password: unexpected error = %v", err)
	}
}

func TestChangePassword_InvalidatesOtherSessions(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)
	ctx := context.Background()

	// Login twice to create two sessions.
	result1, _ := svc.Login(ctx, "127.0.0.1", "admin", testPassword)
	result2, _ := svc.Login(ctx, "127.0.0.2", "admin", testPassword)

	// Change password using session 1.
	newPass := "newpassword1234"
	if err := svc.ChangePassword(ctx, result1.User.ID, result1.SessionToken, testPassword, newPass); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	// Session 1 (caller) should still work.
	req1 := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req1.AddCookie(&http.Cookie{Name: SessionCookieName, Value: result1.SessionToken})
	if _, err := svc.Authenticate(req1); err != nil {
		t.Errorf("caller session should remain valid: %v", err)
	}

	// Session 2 (other) should be invalidated.
	req2 := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req2.AddCookie(&http.Cookie{Name: SessionCookieName, Value: result2.SessionToken})
	_, err := svc.Authenticate(req2)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("other session: error = %v, want %v", err, ErrUnauthenticated)
	}
}

func TestChangePassword_WrongCurrent_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)

	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	err := svc.ChangePassword(context.Background(), result.User.ID, result.SessionToken, "wrongcurrent1", "newpassword1234")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestChangePassword_TooShort_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)

	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	err := svc.ChangePassword(context.Background(), result.User.ID, result.SessionToken, testPassword, "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("error = %v, want %v", err, ErrPasswordTooShort)
	}
}

func TestChangePassword_TooLong_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)

	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	longPass := strings.Repeat("a", MaxPasswordLength+1)
	err := svc.ChangePassword(context.Background(), result.User.ID, result.SessionToken, testPassword, longPass)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("error = %v, want %v", err, ErrPasswordTooLong)
	}
}

// --- ResetPassword ---

func TestResetPassword_Success_InvalidatesSessions(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)
	token := loginTestUser(t, svc)

	newPass := "resetpassword12"
	if err := svc.ResetPassword(context.Background(), "admin", newPass); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}

	// Old session should be invalid.
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	_, err := svc.Authenticate(req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("old session: error = %v, want %v", err, ErrUnauthenticated)
	}

	// New password should work.
	if _, err := svc.Login(context.Background(), "127.0.0.1", "admin", newPass); err != nil {
		t.Errorf("new password: unexpected error = %v", err)
	}
}

func TestResetPassword_UserNotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	err := svc.ResetPassword(context.Background(), "nobody", "newpassword1234")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestResetPassword_TooShort_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	createTestUser(t, svc)

	err := svc.ResetPassword(context.Background(), "admin", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("error = %v, want %v", err, ErrPasswordTooShort)
	}
}

// --- GenerateToken ---

func TestGenerateToken_ReturnsUniqueTokens(t *testing.T) {
	t.Parallel()

	t1, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	t2, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if len(t1) != 64 {
		t.Errorf("token length = %d, want 64", len(t1))
	}
	if t1 == t2 {
		t.Error("consecutive tokens should be unique")
	}
}

// --- Context helpers ---

func TestContextUser_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if got := ContextUser(ctx); got != nil {
		t.Error("expected nil user from empty context")
	}

	user := &User{ID: 42, Username: "admin"}
	ctx = WithUser(ctx, user)
	got := ContextUser(ctx)
	if got == nil || got.ID != 42 || got.Username != "admin" {
		t.Errorf("ContextUser() = %v, want %v", got, user)
	}
}
