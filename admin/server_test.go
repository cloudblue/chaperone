// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandleHealth_ReturnsOK_WithJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	// Act
	s.handleHealth(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if body := rec.Body.String(); body != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", body, `{"status":"ok"}`)
	}
}

func TestSPAHandler_Root_ServesIndexHTML(t *testing.T) {
	t.Parallel()

	// Arrange
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>app</html>")},
	}
	handler := spaHandler(assets)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Errorf("body = %q, want %q", body, "<html>app</html>")
	}
}

func TestSPAHandler_UnknownRoute_FallsBackToIndex(t *testing.T) {
	t.Parallel()

	// Arrange
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa</html>")},
	}
	handler := spaHandler(assets)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/some-page", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "<html>spa</html>" {
		t.Errorf("body = %q, want %q", body, "<html>spa</html>")
	}
}

func TestSecurityHeaders_SetOnAllResponses(t *testing.T) {
	t.Parallel()

	// Arrange
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeaders(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	tests := []struct {
		header string
		want   string
	}{
		{"Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "camera=(), microphone=(), geolocation=()"},
	}
	for _, tt := range tests {
		if got := rec.Header().Get(tt.header); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestSPAHandler_UnmatchedAPIRoute_Returns404(t *testing.T) {
	t.Parallel()

	// Arrange
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa</html>")},
	}
	handler := spaHandler(assets)
	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSPAHandler_ExistingFile_ServesFile(t *testing.T) {
	t.Parallel()

	// Arrange
	assets := fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<html>app</html>")},
		"assets/style.css": &fstest.MapFile{Data: []byte("body{}")},
	}
	handler := spaHandler(assets)
	req := httptest.NewRequest(http.MethodGet, "/assets/style.css", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "body{}" {
		t.Errorf("body = %q, want %q", body, "body{}")
	}
}
