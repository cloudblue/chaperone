// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockAuthenticator struct {
	user *User
	err  error
}

func (m *mockAuthenticator) Authenticate(_ *http.Request) (*User, error) {
	return m.user, m.err
}

func echoUserHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := ContextUser(r.Context())
		if user != nil {
			w.Header().Set("X-User", user.Username)
		}
		w.WriteHeader(http.StatusOK)
	})
}

// --- RequireAuth ---

func TestRequireAuth_ProtectedRoute_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	handler := RequireAuth(&mockAuthenticator{err: ErrUnauthenticated}, echoUserHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var resp middlewareError
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Error.Code != "UNAUTHORIZED" {
		t.Errorf("code = %q, want %q", resp.Error.Code, "UNAUTHORIZED")
	}
}

func TestRequireAuth_ProtectedRoute_Authenticated_PassesThrough(t *testing.T) {
	t.Parallel()

	user := &User{ID: 1, Username: "admin"}
	handler := RequireAuth(&mockAuthenticator{user: user}, echoUserHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-User"); got != "admin" {
		t.Errorf("X-User = %q, want %q", got, "admin")
	}
}

func TestRequireAuth_LoginRoute_SkipsAuth(t *testing.T) {
	t.Parallel()

	handler := RequireAuth(&mockAuthenticator{err: ErrUnauthenticated}, echoUserHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (login should skip auth)", rec.Code, http.StatusOK)
	}
}

func TestRequireAuth_HealthRoute_SkipsAuth(t *testing.T) {
	t.Parallel()

	handler := RequireAuth(&mockAuthenticator{err: ErrUnauthenticated}, echoUserHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (health should skip auth)", rec.Code, http.StatusOK)
	}
}

func TestRequireAuth_SPARoute_SkipsAuth(t *testing.T) {
	t.Parallel()

	handler := RequireAuth(&mockAuthenticator{err: ErrUnauthenticated}, echoUserHandler())
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (non-API should skip auth)", rec.Code, http.StatusOK)
	}
}

// --- CSRFProtection ---

func TestCSRF_SafeMethod_SkipsCheck(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (GET should skip CSRF)", rec.Code, http.StatusOK)
	}
}

func TestCSRF_LoginRoute_SkipsCheck(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (login should skip CSRF)", rec.Code, http.StatusOK)
	}
}

func TestCSRF_WriteRequest_MissingCookie_Returns403(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/instances", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRF_WriteRequest_MissingHeader_Returns403(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "token123"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRF_WriteRequest_MismatchedToken_Returns403(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "token123"})
	req.Header.Set(CSRFHeaderName, "different-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRF_WriteRequest_ValidToken_PassesThrough(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/instances", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "token123"})
	req.Header.Set(CSRFHeaderName, "token123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestCSRF_DeleteRequest_RequiresToken(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodDelete, "/api/instances/1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (DELETE should require CSRF)", rec.Code, http.StatusForbidden)
	}
}

func TestCSRF_PutRequest_ValidToken_PassesThrough(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodPut, "/api/user/password", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "csrf-val"})
	req.Header.Set(CSRFHeaderName, "csrf-val")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestCSRF_NonAPIRoute_SkipsCheck(t *testing.T) {
	t.Parallel()

	handler := CSRFProtection(echoUserHandler())
	req := httptest.NewRequest(http.MethodPost, "/some/form", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (non-API should skip CSRF)", rec.Code, http.StatusOK)
	}
}
