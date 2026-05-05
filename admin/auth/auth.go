// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/cloudblue/chaperone/admin/store"
)

// Cookie and header names used by the auth system.
const (
	SessionCookieName = "session"
	CSRFCookieName    = "csrf_token"
	CSRFHeaderName    = "X-CSRF-Token"
	MinPasswordLength = 12
	MaxPasswordLength = 72 // bcrypt silently truncates beyond this
	MaxUsernameLength = 64
)

// Sentinel errors for authentication operations.
var (
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrPasswordTooShort   = errors.New("password too short")
	ErrPasswordTooLong    = errors.New("password too long")
	ErrInvalidUsername    = errors.New("invalid username")
	ErrRateLimited        = errors.New("rate limited")
	ErrSessionExpired     = errors.New("session expired")
)

// dummyHash is a pre-computed bcrypt hash used when a user is not found,
// to prevent timing-based username enumeration.
//
//nolint:errcheck // bcrypt.GenerateFromPassword with DefaultCost never fails
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)

// Authenticator validates a request and returns the authenticated user.
// This interface enables future auth backends (OIDC, etc.) without
// changing middleware or handlers.
type Authenticator interface {
	Authenticate(r *http.Request) (*User, error)
}

// User represents an authenticated portal user.
type User struct {
	ID       int64
	Username string
}

// LoginResult holds the outcome of a successful login.
type LoginResult struct {
	SessionToken string // #nosec G117 -- this is a session token, not a hardcoded secret
	User         User
}

// Service implements local authentication using SQLite-backed users
// with bcrypt password hashing and session cookies.
type Service struct {
	store       *store.Store
	limiter     *RateLimiter
	maxAge      time.Duration
	idleTimeout time.Duration
}

// NewService creates an auth service with the given session parameters.
func NewService(st *store.Store, maxAge, idleTimeout time.Duration) *Service {
	return &Service{
		store:       st,
		limiter:     NewRateLimiter(5, time.Minute),
		maxAge:      maxAge,
		idleTimeout: idleTimeout,
	}
}

// SweepRateLimiter removes expired entries from the rate limiter.
func (s *Service) SweepRateLimiter() {
	s.limiter.Sweep()
}

// Authenticate validates the session cookie on an HTTP request.
// It checks absolute TTL, idle timeout, and touches the session.
func (s *Service) Authenticate(r *http.Request) (*User, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, ErrUnauthenticated
	}

	rawToken := cookie.Value
	sess, err := s.store.GetSessionByToken(r.Context(), rawToken)
	if errors.Is(err, store.ErrSessionNotFound) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("validating session: %w", err)
	}

	now := time.Now()
	if now.After(sess.ExpiresAt) {
		if delErr := s.store.DeleteSession(r.Context(), rawToken); delErr != nil {
			slog.Error("deleting expired session", "error", delErr)
		}
		return nil, ErrSessionExpired
	}
	if now.Sub(sess.LastActiveAt) > s.idleTimeout {
		if delErr := s.store.DeleteSession(r.Context(), rawToken); delErr != nil {
			slog.Error("deleting idle session", "error", delErr)
		}
		return nil, ErrSessionExpired
	}

	if touchErr := s.store.TouchSession(r.Context(), rawToken); touchErr != nil {
		slog.Error("touching session", "error", touchErr)
	}

	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		return nil, fmt.Errorf("getting user for session: %w", err)
	}

	return &User{ID: user.ID, Username: user.Username}, nil
}

// Login authenticates credentials and creates a new session.
// It enforces rate limiting per IP and uses constant-time comparison
// to prevent username enumeration.
func (s *Service) Login(ctx context.Context, ip, username, password string) (*LoginResult, error) {
	if !s.limiter.Allow(ip) {
		return nil, ErrRateLimited
	}

	user, err := s.store.GetUserByUsername(ctx, username)
	if errors.Is(err, store.ErrUserNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		s.limiter.Record(ip)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("looking up user: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		s.limiter.Record(ip)
		return nil, ErrInvalidCredentials
	}

	s.limiter.Reset(ip)

	token, err := GenerateToken(32)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(s.maxAge)
	if err := s.store.CreateSession(ctx, user.ID, token, expiresAt); err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return &LoginResult{
		SessionToken: token,
		User:         User{ID: user.ID, Username: user.Username},
	}, nil
}

// Logout invalidates a session by its token.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, token)
}

// ChangePassword verifies the current password, updates to a new one,
// and invalidates all sessions except the caller's.
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentToken, currentPassword, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword))
	if err != nil {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	if err := s.store.UpdateUserPassword(ctx, userID, string(hash)); err != nil {
		return err
	}

	if err := s.store.DeleteOtherSessions(ctx, userID, currentToken); err != nil {
		return fmt.Errorf("invalidating other sessions: %w", err)
	}

	return nil
}

// CreateUser creates a new portal user (CLI operation).
func (s *Service) CreateUser(ctx context.Context, username, password string) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	_, err = s.store.CreateUser(ctx, username, string(hash))
	return err
}

// ResetPassword changes a user's password and invalidates all their sessions (CLI operation).
func (s *Service) ResetPassword(ctx context.Context, username, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}

	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("looking up user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	if err := s.store.UpdateUserPassword(ctx, user.ID, string(hash)); err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	if err := s.store.DeleteUserSessions(ctx, user.ID); err != nil {
		return fmt.Errorf("invalidating sessions: %w", err)
	}

	return nil
}

func validatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

func validateUsername(username string) error {
	if username == "" || len(username) > MaxUsernameLength {
		return ErrInvalidUsername
	}
	for _, r := range username {
		if r < 0x20 || r > 0x7E {
			return ErrInvalidUsername
		}
	}
	return nil
}

// GenerateToken returns a cryptographically random hex-encoded token.
func GenerateToken(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

type contextKey struct{}

// WithUser stores an authenticated user in the request context.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}

// ContextUser extracts the authenticated user from a request context.
// Returns nil if no user is present (unauthenticated request).
func ContextUser(ctx context.Context) *User {
	u, _ := ctx.Value(contextKey{}).(*User)
	return u
}
