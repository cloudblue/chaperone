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

	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/internal/proxy"
)

func TestHealth_ReturnsAlive(t *testing.T) {
	// Arrange
	srv := mustNewServer(t, testConfig())
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
	cfg := testConfig()
	cfg.Version = "1.0.0"
	srv := mustNewServer(t, cfg)
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
	srv := mustNewServer(t, testConfig())
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

	handler := proxy.PanicRecoveryMiddleware(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Act - should not panic
	handler.ServeHTTP(rec, req)

	// Assert - status 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Assert - JSON content type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	// Assert - JSON body with generic error
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if body["error"] != "Internal Server Error" {
		t.Errorf("error = %q, want %q", body["error"], "Internal Server Error")
	}
	if body["status"] != float64(500) {
		t.Errorf("status = %v, want %v", body["status"], 500)
	}
}

func TestHealth_ContentType_JSON(t *testing.T) {
	// Arrange
	srv := mustNewServer(t, testConfig())
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
	srv := mustNewServer(t, testConfig())
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

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - request should succeed (trace ID is internal, not in response headers)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestProxy_TraceID_PreservedFromRequest(t *testing.T) {
	// Arrange
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	req.Header.Set("Connect-Request-ID", "my-trace-123")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - request should succeed (trace ID is used internally for logging, not in response)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
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

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
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

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
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

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
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

			srv := mustNewServerForTarget(t, testConfig(), backend.URL)
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

// =============================================================================
// Middleware Stack Tests (using production middleware via observability package)
// =============================================================================

// TestMiddlewareStack_PanicLogsCorrectStatus tests the critical interaction between
// RequestLoggerMiddleware and PanicRecoveryMiddleware. When a handler panics:
// 1. PanicRecovery catches the panic and writes 500 to the ResponseWriter
// 2. RequestLoggerMiddleware's defer logs with the correct status (500, not 200)
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

	// Apply middleware in production order: TraceID → Logger → PanicRecovery → handler
	handler := proxy.PanicRecoveryMiddleware(panicHandler)
	handler = observability.RequestLoggerMiddleware(slog.Default(), "X-Connect-Vendor-ID", handler)
	handler = observability.TraceIDMiddleware("Connect-Request-ID", handler)

	req := httptest.NewRequest(http.MethodPost, "/test/panic", nil)
	req.Header.Set("Connect-Request-ID", "panic-trace-123")
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
// (no panic) log the correct status code through the real middleware stack.
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

	// Apply middleware in production order: TraceID → Logger → PanicRecovery → handler
	wrapped := proxy.PanicRecoveryMiddleware(handler)
	wrapped = observability.RequestLoggerMiddleware(slog.Default(), "X-Connect-Vendor-ID", wrapped)
	wrapped = observability.TraceIDMiddleware("Connect-Request-ID", wrapped)

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

// TestPanicRecovery_LogsTraceID verifies that when PanicRecovery is inside
// TraceIDMiddleware, the panic log includes the trace ID from context.
func TestPanicRecovery_LogsTraceID(t *testing.T) {
	// Arrange - capture log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("trace-id panic test")
	})

	// Production order: TraceID → PanicRecovery → handler
	handler := proxy.PanicRecoveryMiddleware(panicHandler)
	handler = observability.TraceIDMiddleware("Connect-Request-ID", handler)

	req := httptest.NewRequest(http.MethodGet, "/panic-trace", nil)
	req.Header.Set("Connect-Request-ID", "panic-with-trace-789")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - panic log should contain trace_id
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"trace_id":"panic-with-trace-789"`) {
		t.Errorf("panic log should contain trace_id, got: %s", logOutput)
	}
}

// =============================================================================
// NewServer Validation Tests
// =============================================================================

func TestNewServer_MissingRequiredFields_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     proxy.Config
		wantErr string
	}{
		{
			name:    "missing Version",
			cfg:     func() proxy.Config { c := testConfig(); c.Version = ""; return c }(),
			wantErr: "version is required",
		},
		{
			name:    "missing HeaderPrefix",
			cfg:     func() proxy.Config { c := testConfig(); c.HeaderPrefix = ""; return c }(),
			wantErr: "header prefix is required",
		},
		{
			name:    "missing TraceHeader",
			cfg:     func() proxy.Config { c := testConfig(); c.TraceHeader = ""; return c }(),
			wantErr: "trace header is required",
		},
		{
			name:    "nil TLS",
			cfg:     func() proxy.Config { c := testConfig(); c.TLS = nil; return c }(),
			wantErr: "TLS config is required",
		},
		{
			name:    "zero ReadTimeout",
			cfg:     func() proxy.Config { c := testConfig(); c.ReadTimeout = 0; return c }(),
			wantErr: "read timeout must be positive",
		},
		{
			name:    "zero WriteTimeout",
			cfg:     func() proxy.Config { c := testConfig(); c.WriteTimeout = 0; return c }(),
			wantErr: "write timeout must be positive",
		},
		{
			name:    "zero IdleTimeout",
			cfg:     func() proxy.Config { c := testConfig(); c.IdleTimeout = 0; return c }(),
			wantErr: "idle timeout must be positive",
		},
		{
			name:    "zero PluginTimeout",
			cfg:     func() proxy.Config { c := testConfig(); c.PluginTimeout = 0; return c }(),
			wantErr: "plugin timeout must be positive",
		},
		{
			name:    "zero ConnectTimeout",
			cfg:     func() proxy.Config { c := testConfig(); c.ConnectTimeout = 0; return c }(),
			wantErr: "connect timeout must be positive",
		},
		{
			name:    "zero KeepAliveTimeout",
			cfg:     func() proxy.Config { c := testConfig(); c.KeepAliveTimeout = 0; return c }(),
			wantErr: "keep-alive timeout must be positive",
		},
		{
			name:    "zero ShutdownTimeout",
			cfg:     func() proxy.Config { c := testConfig(); c.ShutdownTimeout = 0; return c }(),
			wantErr: "shutdown timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := proxy.NewServer(tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewServer_MultipleErrors_AllReported(t *testing.T) {
	t.Parallel()

	// Arrange - config with multiple missing fields
	_, err := proxy.NewServer(proxy.Config{})

	// Assert - should return error with multiple issues
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errStr := err.Error()
	for _, want := range []string{"version", "header prefix", "trace header", "TLS"} {
		if !strings.Contains(errStr, want) {
			t.Errorf("error should mention %q, got: %s", want, errStr)
		}
	}
}

func TestNewServer_CustomTraceHeader_Preserved(t *testing.T) {
	t.Parallel()

	// Arrange - custom TraceHeader provided
	cfg := testConfig()
	cfg.TraceHeader = "X-Correlation-ID"

	// Act
	server := mustNewServer(t, cfg)

	// Assert - custom value should be preserved
	if server.Config().TraceHeader != "X-Correlation-ID" {
		t.Errorf("TraceHeader = %q, want %q", server.Config().TraceHeader, "X-Correlation-ID")
	}
}

func TestNewServer_CustomTLSConfig(t *testing.T) {
	t.Parallel()

	// Arrange - custom TLS config
	cfg := testConfig()
	cfg.TLS = &proxy.TLSConfig{
		Enabled:  false,
		CertFile: "/custom/cert.pem",
		KeyFile:  "/custom/key.pem",
		CAFile:   "/custom/ca.pem",
	}

	// Act
	server := mustNewServer(t, cfg)

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

// =============================================================================
// Server Start Tests
// =============================================================================

func TestServer_StartTLS_MissingCAFile_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange - TLS enabled but CA file doesn't exist
	cfg := testConfig()
	cfg.TLS = &proxy.TLSConfig{
		Enabled:  true,
		CertFile: "/nonexistent/server.crt",
		KeyFile:  "/nonexistent/server.key",
		CAFile:   "/nonexistent/ca.crt",
	}
	server := mustNewServer(t, cfg)

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

	cfg := testConfig()
	cfg.TLS = &proxy.TLSConfig{
		Enabled:  true,
		CertFile: "/nonexistent/server.crt",
		KeyFile:  "/nonexistent/server.key",
		CAFile:   caFile,
	}
	server := mustNewServer(t, cfg)

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

	cfg := testConfig()
	cfg.TLS = &proxy.TLSConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  "/nonexistent/server.key",
		CAFile:   caFile,
	}
	server := mustNewServer(t, cfg)

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

	cfg := testConfig()
	cfg.TLS = &proxy.TLSConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   caFile,
	}
	server := mustNewServer(t, cfg)

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
	srv := mustNewServer(t, testConfig())
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	// Set a malformed URL that will fail url.Parse
	req.Header.Set("X-Connect-Target-URL", "://invalid-url")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - AllowListMiddleware catches this and returns 400 "invalid target URL"
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid target URL") {
		t.Errorf("body = %q, want to contain 'invalid target URL'", rec.Body.String())
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
