// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/admin/auth"
)

const testPassword = "securepassword12"

func newTestAuthMux(t *testing.T) (*http.ServeMux, *auth.Service) {
	t.Helper()
	st := openTestStore(t)
	svc := auth.NewService(st, 24*time.Hour, 2*time.Hour)
	h := NewAuthHandler(svc, false, 24*time.Hour)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, svc
}

func createTestUser(t *testing.T, svc *auth.Service) {
	t.Helper()
	if err := svc.CreateUser(context.Background(), "admin", testPassword); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
}

// --- Login ---

func TestLogin_Success_Returns200WithCookies(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)

	body := `{"username":"admin","password":"` + testPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.User.Username != "admin" {
		t.Errorf("username = %q, want %q", resp.User.Username, "admin")
	}

	cookies := rec.Result().Cookies()
	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case auth.SessionCookieName:
			sessionCookie = c
		case auth.CSRFCookieName:
			csrfCookie = c
		}
	}

	if sessionCookie == nil {
		t.Fatal("missing session cookie")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if sessionCookie.Secure {
		t.Error("session cookie should not be Secure in test (secureCookies=false)")
	}

	if csrfCookie == nil {
		t.Fatal("missing CSRF cookie")
	}
	if csrfCookie.HttpOnly {
		t.Error("CSRF cookie should NOT be HttpOnly")
	}
}

func TestLogin_WrongPassword_Returns401(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)

	body := `{"username":"admin","password":"wrongpassword1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestLogin_MissingFields_Returns400(t *testing.T) {
	t.Parallel()
	mux, _ := newTestAuthMux(t)

	body := `{"username":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogin_RateLimited_Returns429(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)

	for range 5 {
		body := `{"username":"admin","password":"badpassword00"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}

	body := `{"username":"admin","password":"` + testPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want %q", got, "60")
	}
}

// --- Logout ---

func TestLogout_Returns204_ClearsCookies(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)

	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: result.SessionToken})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.MaxAge != -1 {
			t.Error("session cookie should be cleared (MaxAge=-1)")
		}
		if c.Name == auth.CSRFCookieName && c.MaxAge != -1 {
			t.Error("CSRF cookie should be cleared (MaxAge=-1)")
		}
	}
}

// --- ChangePassword ---

func TestChangePassword_Success_Returns204(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)
	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	body := `{"current_password":"` + testPassword + `","new_password":"newpassword1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/user/password", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: result.SessionToken})
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{
		ID:       result.User.ID,
		Username: result.User.Username,
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestChangePassword_WrongCurrent_Returns401(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)
	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	body := `{"current_password":"wrongcurrent1","new_password":"newpassword1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/user/password", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: result.SessionToken})
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{
		ID:       result.User.ID,
		Username: result.User.Username,
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestChangePassword_TooShort_Returns400(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)
	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	body := `{"current_password":"` + testPassword + `","new_password":"short"}`
	req := httptest.NewRequest(http.MethodPut, "/api/user/password", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: result.SessionToken})
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{
		ID:       result.User.ID,
		Username: result.User.Username,
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChangePassword_NoUser_Returns401(t *testing.T) {
	t.Parallel()
	mux, _ := newTestAuthMux(t)

	body := `{"current_password":"old","new_password":"newpassword1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/user/password", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- Me ---

func TestMe_Authenticated_Returns200(t *testing.T) {
	t.Parallel()
	mux, svc := newTestAuthMux(t)
	createTestUser(t, svc)
	result, _ := svc.Login(context.Background(), "127.0.0.1", "admin", testPassword)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{
		ID:       result.User.ID,
		Username: result.User.Username,
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.User.Username != "admin" {
		t.Errorf("username = %q, want %q", resp.User.Username, "admin")
	}
}

func TestMe_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()
	mux, _ := newTestAuthMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
