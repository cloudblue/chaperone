// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/internal/proxy"
)

func TestHealth_ReturnsAlive(t *testing.T) {
	// Arrange
	srv := proxy.NewServer(proxy.Config{
		Addr: ":0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/_ops/health", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["status"] != "alive" {
		t.Errorf("status = %q, want %q", response["status"], "alive")
	}
}

func TestVersion_ReturnsVersionInfo(t *testing.T) {
	// Arrange
	srv := proxy.NewServer(proxy.Config{
		Addr:    ":0",
		Version: "1.0.0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/_ops/version", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["version"] != "1.0.0" {
		t.Errorf("version = %q, want %q", response["version"], "1.0.0")
	}
}

func TestProxy_MissingTargetURL_Returns400(t *testing.T) {
	// Arrange
	srv := proxy.NewServer(proxy.Config{
		Addr: ":0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	// Missing X-Connect-Target-URL header
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestProxy_WrongMethod_Returns405(t *testing.T) {
	// Arrange
	srv := proxy.NewServer(proxy.Config{
		Addr: ":0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestPanicRecovery_CatchesPanic_Returns500(t *testing.T) {
	// Arrange
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := proxy.WithPanicRecovery(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Act - should not panic
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestServer_ConfigDefaults(t *testing.T) {
	// Arrange & Act
	srv := proxy.NewServer(proxy.Config{
		Addr: ":8080",
	})

	// Assert - verify defaults are applied
	config := srv.Config()

	if config.ReadTimeout == 0 {
		t.Error("ReadTimeout should have a default value")
	}
	if config.WriteTimeout == 0 {
		t.Error("WriteTimeout should have a default value")
	}
	if config.IdleTimeout == 0 {
		t.Error("IdleTimeout should have a default value")
	}
	if config.PluginTimeout == 0 {
		t.Error("PluginTimeout should have a default value")
	}
}

func TestServer_ConfigTimeout_DefaultValues(t *testing.T) {
	// Arrange & Act
	srv := proxy.NewServer(proxy.Config{
		Addr: ":8080",
	})
	config := srv.Config()

	// Assert - verify safe defaults per Design Spec
	tests := []struct {
		name    string
		got     time.Duration
		wantMin time.Duration
		wantMax time.Duration
	}{
		{"ReadTimeout", config.ReadTimeout, 1 * time.Second, 30 * time.Second},
		{"WriteTimeout", config.WriteTimeout, 10 * time.Second, 60 * time.Second},
		{"IdleTimeout", config.IdleTimeout, 30 * time.Second, 300 * time.Second},
		{"PluginTimeout", config.PluginTimeout, 5 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got < tt.wantMin || tt.got > tt.wantMax {
				t.Errorf("%s = %v, want between %v and %v", tt.name, tt.got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestHealth_ContentType_JSON(t *testing.T) {
	// Arrange
	srv := proxy.NewServer(proxy.Config{Addr: ":0"})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/_ops/health", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
}

func TestUnknownRoute_Returns404(t *testing.T) {
	// Arrange
	srv := proxy.NewServer(proxy.Config{Addr: ":0"})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/unknown/route", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestLogging_IncludesTraceID verifies that trace ID is propagated in logs.
// This is tested indirectly through the response header.
func TestProxy_TraceID_GeneratedWhenMissing(t *testing.T) {
	// Arrange
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := proxy.NewServer(proxy.Config{
		Addr: ":0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - trace ID should be in response header
	traceID := rec.Header().Get("X-Trace-ID")
	if traceID == "" {
		t.Error("expected X-Trace-ID header to be set")
	}
}

func TestProxy_TraceID_PreservedFromRequest(t *testing.T) {
	// Arrange
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := proxy.NewServer(proxy.Config{
		Addr: ":0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	req.Header.Set("Connect-Request-ID", "my-trace-123")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - trace ID should match the one from request
	traceID := rec.Header().Get("X-Trace-ID")
	if traceID != "my-trace-123" {
		t.Errorf("X-Trace-ID = %q, want %q", traceID, "my-trace-123")
	}
}

func TestProxy_ResponseSanitizer_StripsAuthHeaders(t *testing.T) {
	// Arrange - backend that returns Authorization in response (should be stripped)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Authorization", "Bearer leaked-token")
		w.Header().Set("X-API-Key", "leaked-key")
		w.Header().Set("X-Safe-Header", "keep-this")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := proxy.NewServer(proxy.Config{
		Addr: ":0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - sensitive headers should be stripped
	if rec.Header().Get("Authorization") != "" {
		t.Error("Authorization header should be stripped from response")
	}
	if rec.Header().Get("X-API-Key") != "" {
		t.Error("X-API-Key header should be stripped from response")
	}
	// Safe headers should be preserved
	if rec.Header().Get("X-Safe-Header") != "keep-this" {
		t.Error("X-Safe-Header should be preserved")
	}
}

func TestNewServer_NilPlugin_UsesNoopPlugin(t *testing.T) {
	// Arrange - server without plugin should still work
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should receive request even without plugin
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "OK")
	}))
	defer backend.Close()

	srv := proxy.NewServer(proxy.Config{
		Addr: ":0",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - should succeed (no credential injection, but forwarding works)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestProxy_TargetURLWithPath_PreservesPath(t *testing.T) {
	// Arrange - backend that verifies the path is preserved
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := proxy.NewServer(proxy.Config{Addr: ":0"})
	handler := srv.Handler()

	// Target URL with a specific path
	targetURL := backend.URL + "/api/v1/resource"
	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", targetURL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedPath != "/api/v1/resource" {
		t.Errorf("path = %q, want %q", receivedPath, "/api/v1/resource")
	}
}

func TestRequestLogging_LogsRequestWithLatency(t *testing.T) {
	// Arrange
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	logged := proxy.WithRequestLogging(handler)

	req := httptest.NewRequest(http.MethodPost, "/test/path", nil)
	req.Header.Set("X-Trace-ID", "trace-abc")
	rec := httptest.NewRecorder()

	// Act
	logged.ServeHTTP(rec, req)

	// Assert - request should complete successfully
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestRequestLogging_UsesConnectRequestID_WhenNoTraceID(t *testing.T) {
	// Arrange
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logged := proxy.WithRequestLogging(handler)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Connect-Request-ID", "connect-trace-123")
	rec := httptest.NewRecorder()

	// Act
	logged.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestMiddlewareStack_PanicLogsCorrectStatus tests the critical interaction between
// WithRequestLogging and WithPanicRecovery middlewares. When a handler panics:
// 1. PanicRecovery catches the panic and writes 500 to the wrapped ResponseWriter
// 2. RequestLogging's defer logs with the correct status (500, not default 200)
func TestMiddlewareStack_PanicLogsCorrectStatus(t *testing.T) {
	// Arrange - capture log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	// Handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("intentional test panic")
	})

	// Apply middleware in the same order as production (logging wraps panic recovery)
	handler := proxy.WithPanicRecovery(panicHandler)
	handler = proxy.WithRequestLogging(handler)

	req := httptest.NewRequest(http.MethodPost, "/test/panic", nil)
	req.Header.Set("X-Trace-ID", "panic-trace-123")
	rec := httptest.NewRecorder()

	// Act - should not panic (recovered)
	handler.ServeHTTP(rec, req)

	// Assert - response should be 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("response status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Assert - log should contain status 500
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"status":500`) {
		t.Errorf("log should contain status 500, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"trace_id":"panic-trace-123"`) {
		t.Errorf("log should contain trace_id, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"path":"/test/panic"`) {
		t.Errorf("log should contain path, got: %s", logOutput)
	}
}

// TestMiddlewareStack_NormalRequestLogsCorrectStatus verifies that normal requests
// (no panic) still log the correct status code.
func TestMiddlewareStack_NormalRequestLogsCorrectStatus(t *testing.T) {
	// Arrange - capture log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	// Handler that returns 201 Created
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	// Apply middleware in production order
	wrapped := proxy.WithPanicRecovery(handler)
	wrapped = proxy.WithRequestLogging(wrapped)

	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	rec := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rec, req)

	// Assert - response should be 201
	if rec.Code != http.StatusCreated {
		t.Errorf("response status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Assert - log should contain status 201
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"status":201`) {
		t.Errorf("log should contain status 201, got: %s", logOutput)
	}
}

// TestRequestLogging_DeferAlwaysRuns verifies that the logging defer runs
// even when the handler panics (before panic recovery).
func TestRequestLogging_DeferAlwaysRuns(t *testing.T) {
	// Arrange - capture log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	// Handler that panics - without panic recovery, to test defer behavior
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic without recovery")
	})

	// Only RequestLogging, no PanicRecovery - the panic will propagate
	handler := proxy.WithRequestLogging(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/will-panic", nil)
	rec := httptest.NewRecorder()

	// Act - catch the propagating panic
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		handler.ServeHTTP(rec, req)
	}()

	// Assert - panic should have propagated
	if !panicked {
		t.Error("expected panic to propagate")
	}

	// Assert - logging defer should still have run
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "request completed") {
		t.Errorf("logging defer should have run even with panic, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"path":"/will-panic"`) {
		t.Errorf("log should contain path, got: %s", logOutput)
	}
}

// TestMiddlewareStack_LogsLatency verifies that latency is captured correctly.
func TestMiddlewareStack_LogsLatency(t *testing.T) {
	// Arrange - capture log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	// Handler that takes some time
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	handler := proxy.WithRequestLogging(slowHandler)

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - log should contain latency_ms field
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"latency_ms":`) {
		t.Errorf("log should contain latency_ms, got: %s", logOutput)
	}
}
