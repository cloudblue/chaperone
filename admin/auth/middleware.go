// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// RequireAuth wraps an http.Handler and enforces session authentication
// on all /api/* routes except POST /api/login and GET /api/health.
func RequireAuth(auth Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAuth(r) {
			next.ServeHTTP(w, r)
			return
		}

		user, err := auth.Authenticate(r)
		if err != nil {
			slog.Debug("authentication failed", "path", r.URL.Path, "error", err)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
			return
		}

		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
	})
}

// CSRFProtection validates the double-submit cookie pattern on all
// write requests to /api/* (except POST /api/login which has no session yet).
func CSRFProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresCSRF(r) {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(CSRFCookieName)
		if err != nil {
			writeError(w, http.StatusForbidden, "CSRF_ERROR", "Missing CSRF token")
			return
		}

		header := r.Header.Get(CSRFHeaderName)
		if header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
			writeError(w, http.StatusForbidden, "CSRF_ERROR", "Invalid CSRF token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requiresAuth(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/login" {
		return false
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/health" {
		return false
	}
	return true
}

func requiresCSRF(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	if r.URL.Path == "/api/login" {
		return false
	}
	return true
}

type middlewareError struct {
	Error middlewareErrorDetail `json:"error"`
}

type middlewareErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(middlewareError{
		Error: middlewareErrorDetail{Code: code, Message: message},
	}); err != nil {
		slog.Error("writing middleware error response", "error", err)
	}
}
