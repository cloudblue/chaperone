// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudblue/chaperone/internal/observability"
)

func TestAllowListMiddleware_ValidRequest(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/v1/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.example.com/v1/customers")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body := rr.Body.String()
	if body != "OK" {
		t.Errorf("expected body 'OK', got %q", body)
	}
}

func TestAllowListMiddleware_BlockedHost(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for blocked host")
	})

	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://evil.com/data")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "host not allowed") {
		t.Errorf("expected error message about host not allowed, got %q", body)
	}
}

func TestAllowListMiddleware_BlockedPath(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/v1/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for blocked path")
	})

	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.example.com/admin/users")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "path not allowed") {
		t.Errorf("expected error message about path not allowed, got %q", body)
	}
}

func TestAllowListMiddleware_MissingTargetURL(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called when target URL is missing")
	})

	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	// Not setting X-Connect-Target-URL header

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestAllowListMiddleware_CustomHeaderPrefix(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewAllowListMiddleware(allowList, "X-Custom", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Custom-Target-URL", "https://api.example.com/test")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestAllowListMiddleware_EmptyAllowList(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for empty allow list")
	})

	middleware := NewAllowListMiddleware(nil, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.example.com/test")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestAllowListMiddleware_ResponseBody(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for blocked host")
	})

	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://evil.com/data")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	// Check content type is JSON
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", contentType)
	}

	// Read and verify body structure
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	// Should contain error message but not leak sensitive URL details
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "error") {
		t.Errorf("expected JSON with 'error' field, got %q", bodyStr)
	}
}

func TestAllowListMiddleware_InvalidTargetURL(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for invalid URL")
	})

	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	tests := []struct {
		name           string
		targetURL      string
		expectedStatus int
	}{
		{
			name:           "invalid URL scheme",
			targetURL:      "://invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing scheme",
			targetURL:      "api.example.com/test",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "path traversal attempt",
			targetURL:      "https://api.example.com/../secret",
			expectedStatus: http.StatusBadRequest, // Path traversal returns 400
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", tt.targetURL)

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestAllowListMiddleware_DoesNotLeakURLDetails(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/v1/**"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	})

	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	// Test with sensitive-looking URL
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://secret-internal.corp.com/api/key=supersecret123")

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	body := rr.Body.String()

	// Response should NOT contain the secret URL parts
	if strings.Contains(body, "secret-internal") {
		t.Error("response body should not leak host details")
	}
	if strings.Contains(body, "supersecret") {
		t.Error("response body should not leak query parameters")
	}
	if strings.Contains(body, "key=") {
		t.Error("response body should not leak query parameters")
	}
}

func TestAllowListMiddleware_AllMethods(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			called := false
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

			req := httptest.NewRequest(method, "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", "https://api.example.com/test")

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if !called {
				t.Errorf("next handler was not called for method %s", method)
			}
			if rr.Code != http.StatusOK {
				t.Errorf("expected status %d for method %s, got %d", http.StatusOK, method, rr.Code)
			}
		})
	}
}

func TestAllowListMiddleware_ValidationPassed_DebugLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	allowList := map[string][]string{"api.example.com": {"/**"}}
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req = req.WithContext(observability.WithTraceID(req.Context(), "trace-debug-123"))
	req.Header.Set("X-Connect-Target-URL", "https://api.example.com/v1/users")
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, `"msg":"allow list validation passed"`) {
		t.Errorf("expected allow list validation passed debug log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"trace_id":"trace-debug-123"`) {
		t.Errorf("expected trace_id in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"target_host":"api.example.com"`) {
		t.Errorf("expected target_host in log, got: %s", logOutput)
	}
}

func TestAllowListMiddleware_ValidationFailed_HasTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	allowList := map[string][]string{"api.example.com": {"/**"}}
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	})
	middleware := NewAllowListMiddleware(allowList, "X-Connect", nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req = req.WithContext(observability.WithTraceID(req.Context(), "trace-fail-456"))
	req.Header.Set("X-Connect-Target-URL", "https://evil.com/data")
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, `"trace_id":"trace-fail-456"`) {
		t.Errorf("expected trace_id in failure log, got: %s", logOutput)
	}
}

func TestAllowListMiddleware_EmptyAllowListDeniesAll(t *testing.T) {
	// Both nil and empty map should deny all
	testCases := []struct {
		name      string
		allowList map[string][]string
	}{
		{"nil allow list", nil},
		{"empty allow list", map[string][]string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("next handler should not be called")
			})

			middleware := NewAllowListMiddleware(tc.allowList, "X-Connect", nextHandler)

			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", "https://api.example.com/test")

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
			}
		})
	}
}
