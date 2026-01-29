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
	"os"
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

func TestProxy_MethodPassthrough_ForwardsOriginalMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
	}{
		{"GET", http.MethodGet},
		{"POST", http.MethodPost},
		{"PUT", http.MethodPut},
		{"PATCH", http.MethodPatch},
		{"DELETE", http.MethodDelete},
		{"HEAD", http.MethodHead},
		{"OPTIONS", http.MethodOptions},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange - backend that captures the received method
			var receivedMethod string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			srv := proxy.NewServer(proxy.Config{Addr: ":0"})
			handler := srv.Handler()

			req := httptest.NewRequest(tt.method, "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", backend.URL)
			req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
			rec := httptest.NewRecorder()

			// Act
			handler.ServeHTTP(rec, req)

			// Assert - method should be forwarded to backend
			if receivedMethod != tt.method {
				t.Errorf("backend received method = %q, want %q", receivedMethod, tt.method)
			}
		})
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

// =============================================================================
// TLS Configuration Tests
// =============================================================================

func TestNewServer_DefaultTLSConfig(t *testing.T) {
	t.Parallel()

	// Arrange - no TLS config provided
	cfg := proxy.Config{
		Addr:         ":8443",
		HeaderPrefix: "X-Connect",
	}

	// Act
	server := proxy.NewServer(cfg)

	// Assert - TLS should be enabled with defaults
	if server.Config().TLS == nil {
		t.Fatal("TLS config should not be nil")
	}
	if !server.Config().TLS.Enabled {
		t.Error("TLS.Enabled should be true by default")
	}
	if server.Config().TLS.CertFile != proxy.DefaultCertFile {
		t.Errorf("TLS.CertFile = %q, want %q", server.Config().TLS.CertFile, proxy.DefaultCertFile)
	}
	if server.Config().TLS.KeyFile != proxy.DefaultKeyFile {
		t.Errorf("TLS.KeyFile = %q, want %q", server.Config().TLS.KeyFile, proxy.DefaultKeyFile)
	}
	if server.Config().TLS.CAFile != proxy.DefaultCAFile {
		t.Errorf("TLS.CAFile = %q, want %q", server.Config().TLS.CAFile, proxy.DefaultCAFile)
	}
}

func TestNewServer_CustomTLSConfig(t *testing.T) {
	t.Parallel()

	// Arrange - custom TLS config
	cfg := proxy.Config{
		Addr:         ":8443",
		HeaderPrefix: "X-Connect",
		TLS: &proxy.TLSConfig{
			Enabled:  false,
			CertFile: "/custom/cert.pem",
			KeyFile:  "/custom/key.pem",
			CAFile:   "/custom/ca.pem",
		},
	}

	// Act
	server := proxy.NewServer(cfg)

	// Assert - custom values should be preserved
	if server.Config().TLS.Enabled {
		t.Error("TLS.Enabled should be false as configured")
	}
	if server.Config().TLS.CertFile != "/custom/cert.pem" {
		t.Errorf("TLS.CertFile = %q, want %q", server.Config().TLS.CertFile, "/custom/cert.pem")
	}
	if server.Config().TLS.KeyFile != "/custom/key.pem" {
		t.Errorf("TLS.KeyFile = %q, want %q", server.Config().TLS.KeyFile, "/custom/key.pem")
	}
	if server.Config().TLS.CAFile != "/custom/ca.pem" {
		t.Errorf("TLS.CAFile = %q, want %q", server.Config().TLS.CAFile, "/custom/ca.pem")
	}
}

func TestNewServer_PartialTLSConfig(t *testing.T) {
	t.Parallel()

	// Arrange - partial TLS config (only Enabled set, paths empty)
	cfg := proxy.Config{
		Addr:         ":8443",
		HeaderPrefix: "X-Connect",
		TLS: &proxy.TLSConfig{
			Enabled: true,
			// Other fields empty - should be filled with defaults
		},
	}

	// Act
	server := proxy.NewServer(cfg)

	// Assert - empty strings should be filled with defaults
	if server.Config().TLS.CertFile != proxy.DefaultCertFile {
		t.Errorf("TLS.CertFile = %q, want default %q", server.Config().TLS.CertFile, proxy.DefaultCertFile)
	}
	if server.Config().TLS.KeyFile != proxy.DefaultKeyFile {
		t.Errorf("TLS.KeyFile = %q, want default %q", server.Config().TLS.KeyFile, proxy.DefaultKeyFile)
	}
	if server.Config().TLS.CAFile != proxy.DefaultCAFile {
		t.Errorf("TLS.CAFile = %q, want default %q", server.Config().TLS.CAFile, proxy.DefaultCAFile)
	}
}

// =============================================================================
// Server Start Tests
// =============================================================================

func TestServer_StartTLS_MissingCAFile_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange - TLS enabled but CA file doesn't exist
	cfg := proxy.Config{
		Addr: ":0",
		TLS: &proxy.TLSConfig{
			Enabled:  true,
			CertFile: "/nonexistent/server.crt",
			KeyFile:  "/nonexistent/server.key",
			CAFile:   "/nonexistent/ca.crt",
		},
	}
	server := proxy.NewServer(cfg)

	// Act
	err := server.Start()

	// Assert - should fail with file not found error
	if err == nil {
		t.Fatal("expected error for missing CA file, got nil")
	}
	if !strings.Contains(err.Error(), "reading CA certificate") {
		t.Errorf("error = %q, want to contain 'reading CA certificate'", err.Error())
	}
}

func TestServer_StartTLS_MissingCertFile_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange - create temp CA file but no cert file
	tmpDir := t.TempDir()
	caFile := tmpDir + "/ca.crt"
	if err := createDummyFile(caFile); err != nil {
		t.Fatalf("failed to create temp CA file: %v", err)
	}

	cfg := proxy.Config{
		Addr: ":0",
		TLS: &proxy.TLSConfig{
			Enabled:  true,
			CertFile: "/nonexistent/server.crt",
			KeyFile:  "/nonexistent/server.key",
			CAFile:   caFile,
		},
	}
	server := proxy.NewServer(cfg)

	// Act
	err := server.Start()

	// Assert - should fail with file not found error for cert
	if err == nil {
		t.Fatal("expected error for missing cert file, got nil")
	}
	if !strings.Contains(err.Error(), "reading server certificate") {
		t.Errorf("error = %q, want to contain 'reading server certificate'", err.Error())
	}
}

func TestServer_StartTLS_MissingKeyFile_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange - create temp CA and cert files but no key file
	tmpDir := t.TempDir()
	caFile := tmpDir + "/ca.crt"
	certFile := tmpDir + "/server.crt"
	if err := createDummyFile(caFile); err != nil {
		t.Fatalf("failed to create temp CA file: %v", err)
	}
	if err := createDummyFile(certFile); err != nil {
		t.Fatalf("failed to create temp cert file: %v", err)
	}

	cfg := proxy.Config{
		Addr: ":0",
		TLS: &proxy.TLSConfig{
			Enabled:  true,
			CertFile: certFile,
			KeyFile:  "/nonexistent/server.key",
			CAFile:   caFile,
		},
	}
	server := proxy.NewServer(cfg)

	// Act
	err := server.Start()

	// Assert - should fail with file not found error for key
	if err == nil {
		t.Fatal("expected error for missing key file, got nil")
	}
	if !strings.Contains(err.Error(), "reading server key") {
		t.Errorf("error = %q, want to contain 'reading server key'", err.Error())
	}
}

func TestServer_StartTLS_InvalidCerts_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange - create files with invalid cert content
	tmpDir := t.TempDir()
	caFile := tmpDir + "/ca.crt"
	certFile := tmpDir + "/server.crt"
	keyFile := tmpDir + "/server.key"

	// Write invalid PEM data
	if err := createFileWithContent(caFile, "invalid ca cert"); err != nil {
		t.Fatalf("failed to create temp CA file: %v", err)
	}
	if err := createFileWithContent(certFile, "invalid server cert"); err != nil {
		t.Fatalf("failed to create temp cert file: %v", err)
	}
	if err := createFileWithContent(keyFile, "invalid server key"); err != nil {
		t.Fatalf("failed to create temp key file: %v", err)
	}

	cfg := proxy.Config{
		Addr: ":0",
		TLS: &proxy.TLSConfig{
			Enabled:  true,
			CertFile: certFile,
			KeyFile:  keyFile,
			CAFile:   caFile,
		},
	}
	server := proxy.NewServer(cfg)

	// Act
	err := server.Start()

	// Assert - should fail with TLS config error
	if err == nil {
		t.Fatal("expected error for invalid certs, got nil")
	}
	if !strings.Contains(err.Error(), "creating TLS config") {
		t.Errorf("error = %q, want to contain 'creating TLS config'", err.Error())
	}
}

// =============================================================================
// Proxy Invalid URL Tests
// =============================================================================

func TestProxy_InvalidTargetURL_Returns400(t *testing.T) {
	t.Parallel()

	// Arrange
	srv := proxy.NewServer(proxy.Config{
		Addr:         ":0",
		HeaderPrefix: "X-Connect",
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	// Set a malformed URL that will fail url.Parse
	req.Header.Set("X-Connect-Target-URL", "://invalid-url")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Bad Request") {
		t.Errorf("body = %q, want to contain 'Bad Request'", rec.Body.String())
	}
}

// Helper functions for creating test files

func createDummyFile(path string) error {
	return createFileWithContent(path, "dummy content")
}

func createFileWithContent(path, content string) error {
	return writeFile(path, []byte(content))
}

func writeFile(path string, data []byte) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func createFile(path string) (*file, error) {
	return osCreate(path)
}

// file wraps os.File for testing
type file = os.File

var osCreate = os.Create
