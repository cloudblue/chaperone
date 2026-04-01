// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/cloudblue/chaperone/admin/auth"
)

// AuthHandler handles login, logout, and password change endpoints.
type AuthHandler struct {
	auth          *auth.Service
	secureCookies bool
	sessionMaxAge time.Duration
}

// NewAuthHandler creates a handler for auth endpoints.
func NewAuthHandler(authService *auth.Service, secureCookies bool, sessionMaxAge time.Duration) *AuthHandler {
	return &AuthHandler{
		auth:          authService,
		secureCookies: secureCookies,
		sessionMaxAge: sessionMaxAge,
	}
}

// Register mounts auth routes on the given mux.
func (h *AuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/logout", h.logout)
	mux.HandleFunc("GET /api/me", h.me)
	mux.HandleFunc("PUT /api/user/password", h.changePassword)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- request field, not a hardcoded secret
}

type loginResponse struct {
	User loginUser `json:"user"`
}

type loginUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "username and password are required")
		return
	}

	ip := clientIP(r)
	result, err := h.auth.Login(r.Context(), ip, req.Username, req.Password)
	if errors.Is(err, auth.ErrRateLimited) {
		w.Header().Set("Retry-After", "60")
		respondError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many failed login attempts. Try again later.")
		return
	}
	if errors.Is(err, auth.ErrInvalidCredentials) {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid username or password")
		return
	}
	if err != nil {
		slog.Error("login failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Login failed")
		return
	}

	h.setSessionCookie(w, result.SessionToken)
	h.setCSRFCookie(w)

	respondJSON(w, http.StatusOK, loginResponse{
		User: loginUser{
			ID:       result.User.ID,
			Username: result.User.Username,
		},
	})
}

func (h *AuthHandler) me(w http.ResponseWriter, r *http.Request) {
	user := auth.ContextUser(r.Context())
	if user == nil {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	respondJSON(w, http.StatusOK, loginResponse{
		User: loginUser{ID: user.ID, Username: user.Username},
	})
}

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err == nil {
		if logoutErr := h.auth.Logout(r.Context(), cookie.Value); logoutErr != nil {
			slog.Error("logout session deletion", "error", logoutErr)
		}
	}
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *AuthHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	user := auth.ContextUser(r.Context())
	if user == nil {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var req changePasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "current_password and new_password are required")
		return
	}

	err = h.auth.ChangePassword(r.Context(), user.ID, cookie.Value, req.CurrentPassword, req.NewPassword)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Current password is incorrect")
		return
	}
	if errors.Is(err, auth.ErrPasswordTooShort) {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR",
			fmt.Sprintf("Password must be at least %d characters", auth.MinPasswordLength))
		return
	}
	if errors.Is(err, auth.ErrPasswordTooLong) {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR",
			fmt.Sprintf("Password must be at most %d characters", auth.MaxPasswordLength))
		return
	}
	if err != nil {
		slog.Error("password change failed", "user_id", user.ID, "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to change password")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(h.sessionMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) setCSRFCookie(w http.ResponseWriter) {
	token, err := auth.GenerateToken(16)
	if err != nil {
		slog.Error("generating CSRF token", "error", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CSRFCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(h.sessionMaxAge.Seconds()),
		HttpOnly: false,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *AuthHandler) clearCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
