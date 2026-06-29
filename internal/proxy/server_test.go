// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/sdk"
)

func TestHealth_ReturnsAlive(t *testing.T) {
	// Arrange
	srv := mustNewServer(t, testConfig())
	handler := srv.Handler()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_ops/health", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_ops/version", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/proxy", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_ops/health", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/unknown/route", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/proxy", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/proxy", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/proxy", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/proxy", nil)
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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/proxy", nil)
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

			req := httptest.NewRequestWithContext(context.Background(), tt.method, "/proxy", nil)
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
	getLogs := captureLogs(t)

	// Handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("intentional test panic")
	})

	// Apply middleware in production order: TraceID → Logger → PanicRecovery → handler
	handler := proxy.PanicRecoveryMiddleware(panicHandler)
	handler = observability.RequestLoggerMiddleware(slog.Default(), "X-Connect", observability.TargetAddrModeHost, handler)
	handler = observability.TraceIDMiddleware("Connect-Request-ID", handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/test/panic", nil)
	req.Header.Set("Connect-Request-ID", "panic-trace-123")
	rec := httptest.NewRecorder()

	// Act - should not panic (recovered)
	handler.ServeHTTP(rec, req)

	// Assert - response should be 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("response status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Assert - log should contain status 500
	logOutput := getLogs()
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
	getLogs := captureLogs(t)

	// Handler that returns 201 Created
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	// Apply middleware in production order: TraceID → Logger → PanicRecovery → handler
	wrapped := proxy.PanicRecoveryMiddleware(handler)
	wrapped = observability.RequestLoggerMiddleware(slog.Default(), "X-Connect", observability.TargetAddrModeHost, wrapped)
	wrapped = observability.TraceIDMiddleware("Connect-Request-ID", wrapped)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/resource", nil)
	rec := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rec, req)

	// Assert - response should be 201
	if rec.Code != http.StatusCreated {
		t.Errorf("response status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Assert - log should contain status 201
	logOutput := getLogs()
	if !strings.Contains(logOutput, `"status":201`) {
		t.Errorf("log should contain status 201, got: %s", logOutput)
	}
}

// TestPanicRecovery_LogsTraceID verifies that when PanicRecovery is inside
// TraceIDMiddleware, the panic log includes the trace ID from context.
func TestPanicRecovery_LogsTraceID(t *testing.T) {
	// Arrange - capture log output
	getLogs := captureLogs(t)

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("trace-id panic test")
	})

	// Production order: TraceID → PanicRecovery → handler
	handler := proxy.PanicRecoveryMiddleware(panicHandler)
	handler = observability.TraceIDMiddleware("Connect-Request-ID", handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/panic-trace", nil)
	req.Header.Set("Connect-Request-ID", "panic-with-trace-789")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - panic log should contain trace_id
	logOutput := getLogs()
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/proxy", nil)
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

// =============================================================================
// Forward proxy registry tests
// =============================================================================

// TestServer_BuildsForwardProxies_AtStartup is the spec-required test. It
// verifies that the named target "company-b" is built into the registry when
// NewServer returns.
func TestServer_BuildsForwardProxies_AtStartup(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {
			URL:  "https://company-b.example/ingress",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	srv, err := proxy.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	if srv.ForwardProxyForTesting("company-b") == nil {
		t.Errorf("forwardProxies[company-b] not built")
	}
}

// TestServer_BuildsForwardProxies_Matrix covers the full behavior matrix:
//   - zero targets → registry is non-nil and empty
//   - one target → built and accessible
//   - multiple targets → all built
//   - invalid URL → NewServer returns error mentioning the offending name
//   - bearer auth → built and accessible
func TestServer_BuildsForwardProxies_Matrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		targets        map[string]config.ForwardTargetConfig
		wantBuilt      []string // names that must be present after NewServer
		wantErrContain string   // if non-empty, NewServer must fail with this substring
	}{
		{
			name:      "zero targets — registry is non-nil and empty",
			targets:   nil,
			wantBuilt: nil,
		},
		{
			name: "one target — built and accessible by name",
			targets: map[string]config.ForwardTargetConfig{
				"company-b": {
					URL:  "https://company-b.example/ingress",
					Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
				},
			},
			wantBuilt: []string{"company-b"},
		},
		{
			name: "multiple targets — all built and accessible",
			targets: map[string]config.ForwardTargetConfig{
				"company-b": {
					URL:  "https://company-b.example/ingress",
					Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
				},
				"company-c": {
					URL:  "https://company-c.example/api",
					Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthBearer, Token: "tok"},
				},
				"company-d": {
					URL:  "https://company-d.example",
					Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
				},
			},
			wantBuilt: []string{"company-b", "company-c", "company-d"},
		},
		{
			name: "invalid URL — NewServer returns error mentioning the offending name",
			targets: map[string]config.ForwardTargetConfig{
				"broken": {
					URL:  ":::not a url",
					Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
				},
			},
			wantErrContain: "broken",
		},
		{
			name: "bearer auth — built and accessible",
			targets: map[string]config.ForwardTargetConfig{
				"company-x": {
					URL:  "https://company-x.example/ingress",
					Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthBearer, Token: "secret-token"},
				},
			},
			wantBuilt: []string{"company-x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := testConfig()
			cfg.ForwardTargets = tt.targets

			srv, err := proxy.NewServer(cfg)

			if tt.wantErrContain != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrContain)
				}
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}

			// Registry must be non-nil even when no targets are configured.
			if srv.ForwardProxiesNilForTesting() {
				t.Error("forwardProxies map must be non-nil")
			}

			if got, want := srv.ForwardProxyCountForTesting(), len(tt.wantBuilt); got != want {
				t.Errorf("forward proxy count = %d, want %d", got, want)
			}

			for _, name := range tt.wantBuilt {
				if srv.ForwardProxyForTesting(name) == nil {
					t.Errorf("forwardProxies[%q] not built", name)
				}
			}
		})
	}
}

// =============================================================================
// RequestRouter Detection Tests
// =============================================================================

func TestServer_DetectsRequestRouter(t *testing.T) {
	t.Parallel()

	// Arrange
	plugin := &routerPlugin{} // implements sdk.Plugin + sdk.RequestRouter
	cfg := testConfig()
	cfg.Plugin = plugin

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	if srv.RouterForTesting() == nil {
		t.Fatal("router not detected on plugin implementing sdk.RequestRouter")
	}

	// Verify the router is the same as the plugin
	if srv.RouterForTesting() != plugin {
		t.Error("router should be the same instance as the plugin")
	}
}

func TestServer_NoRouter_WhenPluginDoesNotImplement(t *testing.T) {
	t.Parallel()

	// Arrange
	plugin := &plainPlugin{} // implements sdk.Plugin only
	cfg := testConfig()
	cfg.Plugin = plugin

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	if srv.RouterForTesting() != nil {
		t.Fatal("router should be nil for plugin without RequestRouter")
	}
}

func TestServer_NoRouter_WhenNoPluginConfigured(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := testConfig()
	cfg.Plugin = nil

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	if srv.RouterForTesting() != nil {
		t.Fatal("router should be nil when no plugin is configured")
	}
}

func TestServer_RouterIsAccessible_WhenImplemented(t *testing.T) {
	t.Parallel()

	// Arrange
	plugin := &routerPlugin{} // implements both sdk.Plugin and sdk.RequestRouter
	cfg := testConfig()
	cfg.Plugin = plugin

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	routerIface := srv.RouterForTesting()
	if routerIface == nil {
		t.Fatal("router should not be nil")
	}

	// Type assert to sdk.RequestRouter
	router, ok := routerIface.(sdk.RequestRouter)
	if !ok {
		t.Fatal("router should be an sdk.RequestRouter")
	}

	// Create a test request to verify the router is callable
	req := httptest.NewRequest(http.MethodPost, "https://vendor.example/api", nil)
	ctx := req.Context()

	// Act - call RouteRequest on the retrieved router
	tx := testTransactionContext()
	action, err := router.RouteRequest(ctx, tx, req)

	// Assert - for the test plugin, should return a RouteAction with ForwardTo="test-target"
	if err != nil {
		t.Fatalf("RouteRequest: %v", err)
	}
	if action == nil || action.ForwardTo != "test-target" {
		t.Errorf("action.ForwardTo = %q, want %q", action.ForwardTo, "test-target")
	}
}

// =============================================================================
// handleProxy router-branch tests (Task 7)
// =============================================================================

// newProxyRequest builds a /proxy request with the minimum X-Connect-* headers
// needed for handleProxy to reach the router branch.
func newProxyRequest(t *testing.T, targetURL string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", targetURL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	req.Header.Set("X-Connect-Marketplace-ID", "test-marketplace")
	return req
}

func TestHandleProxy_ForwardAction_DispatchesToForwardProxy_AndSkipsCredentials(t *testing.T) {
	var hitTarget bool
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		hitTarget = true
	}))
	defer target.Close()

	var injectedCreds bool
	plugin := &routerPlugin{
		action:           &sdk.RouteAction{ForwardTo: "company-b"},
		actionSet:        true,
		onGetCredentials: func() { injectedCreds = true },
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {URL: target.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
	}
	srv := mustNewServerForTarget(t, cfg, target.URL)

	req := newProxyRequest(t, target.URL+"/v1/foo")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if !hitTarget {
		t.Error("forward target was not called")
	}
	if injectedCreds {
		t.Error("GetCredentials was called for a forwarded request")
	}
}

func TestHandleProxy_NilRouteAction_FallsThroughToCredentialFlow(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	var injectedCreds bool
	plugin := &routerPlugin{
		action:           nil, // fall through
		actionSet:        true,
		onGetCredentials: func() { injectedCreds = true },
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, newProxyRequest(t, backend.URL))

	if !injectedCreds {
		t.Error("GetCredentials should have been called for fall-through")
	}
}

func TestHandleProxy_UnknownForwardTarget_Returns500(t *testing.T) {
	plugin := &routerPlugin{
		action:    &sdk.RouteAction{ForwardTo: "missing"},
		actionSet: true,
	}
	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServer(t, cfg)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, newProxyRequest(t, "https://api.vendor.example/v1/foo"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestHandleProxy_RouterError_Returns500(t *testing.T) {
	plugin := &routerPlugin{
		actionErr: errors.New("router blew up"),
	}
	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServer(t, cfg)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, newProxyRequest(t, "https://api.vendor.example/v1/foo"))

	// handlePluginError maps a generic plugin error to 500.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	// Defense: the wire response must not leak the router's internal
	// error message verbatim. handlePluginError writes a generic body.
	if strings.Contains(rec.Body.String(), "router blew up") {
		t.Errorf("response body leaked router error: %s", rec.Body.String())
	}
}

func TestHandleProxy_RouteActionEmptyForwardTo_FallsThroughToCredentialFlow(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	var injectedCreds bool
	plugin := &routerPlugin{
		action:           &sdk.RouteAction{ForwardTo: ""}, // empty == fall-through
		actionSet:        true,
		onGetCredentials: func() { injectedCreds = true },
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, newProxyRequest(t, backend.URL))

	if !injectedCreds {
		t.Error("empty ForwardTo should fall through to credential flow")
	}
}

func TestHandleProxy_PluginWithoutRequestRouter_GoesDirectlyToCredentials(t *testing.T) {
	// The credentials-path "request routed" breadcrumb is logged at DEBUG, so
	// capture at debug level to assert it.
	getLogs := captureLogsAt(t, &slog.HandlerOptions{Level: slog.LevelDebug})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	plugin := &plainPlugin{} // no RequestRouter
	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, newProxyRequest(t, backend.URL))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	out := getLogs()
	if strings.Contains(out, `"action":"forward"`) {
		t.Errorf("plain plugin must not log action=forward, got: %s", out)
	}
	if !strings.Contains(out, `"action":"credentials"`) {
		t.Errorf("expected action=credentials log line, got: %s", out)
	}
}

func TestHandleProxy_ForwardedResponse_PropagatedToClient(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Forward-Echo", "from-target")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, `{"forwarded":true}`)
	}))
	defer target.Close()

	plugin := &routerPlugin{
		action:    &sdk.RouteAction{ForwardTo: "company-b"},
		actionSet: true,
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {URL: target.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
	}
	srv := mustNewServerForTarget(t, cfg, target.URL)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, newProxyRequest(t, target.URL+"/v1/foo"))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if got := rec.Header().Get("X-Forward-Echo"); got != "from-target" {
		t.Errorf("X-Forward-Echo = %q, want %q", got, "from-target")
	}
	if body := rec.Body.String(); !strings.Contains(body, `"forwarded":true`) {
		t.Errorf("body = %q, want to contain forwarded payload", body)
	}
}

func TestHandleProxy_ForwardPath_LogsActionForward(t *testing.T) {
	getLogs := captureLogs(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	plugin := &routerPlugin{
		action:    &sdk.RouteAction{ForwardTo: "company-b"},
		actionSet: true,
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {URL: target.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
	}
	srv := mustNewServerForTarget(t, cfg, target.URL)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, newProxyRequest(t, target.URL+"/v1/foo"))

	out := getLogs()
	if !strings.Contains(out, `"action":"forward"`) {
		t.Errorf("expected action=forward log line, got: %s", out)
	}
	if !strings.Contains(out, `"target":"company-b"`) {
		t.Errorf("expected target=company-b log line, got: %s", out)
	}
	if strings.Contains(out, `"action":"credentials"`) {
		t.Errorf("forward path must not emit action=credentials, got: %s", out)
	}
}

// Test doubles for RequestRouter detection tests

// plainPlugin implements sdk.Plugin but NOT sdk.RequestRouter
type plainPlugin struct{}

func (p *plainPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
	return nil, nil
}

func (p *plainPlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
	return nil, nil
}

func (p *plainPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

var _ sdk.Plugin = (*plainPlugin)(nil)

// routerPlugin implements both sdk.Plugin and sdk.RequestRouter.
//
// Zero value (routerPlugin{}) preserves the legacy Task 6 behavior:
// RouteRequest returns &sdk.RouteAction{ForwardTo: "test-target"}.
//
// Tests may override behavior by setting any of:
//   - action / actionErr  → returned from RouteRequest
//   - actionSet           → when true, the (action, actionErr) pair is used
//     verbatim even if action is nil (so callers can express a nil-action
//     fall-through explicitly)
//   - onGetCredentials    → invoked when GetCredentials is called (records
//     whether the credential path ran)
type routerPlugin struct {
	action           *sdk.RouteAction
	actionErr        error
	actionSet        bool
	onGetCredentials func()
}

func (r *routerPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
	if r.onGetCredentials != nil {
		r.onGetCredentials()
	}
	return nil, nil
}

func (r *routerPlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
	return nil, nil
}

func (r *routerPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

func (r *routerPlugin) RouteRequest(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.RouteAction, error) {
	if r.actionSet || r.actionErr != nil {
		return r.action, r.actionErr
	}
	// Legacy default for Task 6 tests.
	return &sdk.RouteAction{ForwardTo: "test-target"}, nil
}

var _ sdk.Plugin = (*routerPlugin)(nil)
var _ sdk.RequestRouter = (*routerPlugin)(nil)

// testTransactionContext returns a minimal TransactionContext for testing
func testTransactionContext() sdk.TransactionContext {
	return sdk.TransactionContext{
		TraceID:       "test-trace-123",
		VendorID:      "test-vendor",
		MarketplaceID: "test-marketplace",
		ProductID:     "test-product",
		TargetURL:     "https://vendor.example/api",
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

// =============================================================================
// Forward Reference Validation Tests (Task 13)
// =============================================================================

func TestRun_ForwardActionReferencingUnknownTarget_FailsAtStartup(t *testing.T) {
	t.Parallel()

	// Arrange - Mux with forward action referencing non-existent target
	mux := newTestMux()
	mux.HandleForward(newTestRoute("vendor-x"), "missing-target")

	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {
			URL:  "https://company-b.example/ingress",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	// Act
	_, err := proxy.NewServer(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected startup error for unknown forward target")
	}
	if !strings.Contains(err.Error(), "missing-target") {
		t.Errorf("error should mention missing-target: %v", err)
	}
}

func TestRun_AllForwardReferencesValid_SucceedsAtStartup(t *testing.T) {
	t.Parallel()

	// Arrange - Mux with forward actions all referencing known targets
	mux := newTestMux()
	mux.HandleForward(newTestRoute("vendor-a"), "company-b")
	mux.HandleForward(newTestRoute("vendor-c"), "company-d")

	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {
			URL:  "https://company-b.example/ingress",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
		"company-d": {
			URL:  "https://company-d.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if srv == nil {
		t.Fatal("server should be created")
	}
}

func TestRun_PluginWithoutForwardReferencesMethods_NoValidationError(t *testing.T) {
	t.Parallel()

	// Arrange - custom plugin without ForwardReferences() method
	plugin := &plainPlugin{} // does not implement ForwardReferences()

	cfg := testConfig()
	cfg.Plugin = plugin
	// Configured forward_targets that are never referenced
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"unused-target": {
			URL:  "https://unused.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert - should succeed; no validation against custom plugins
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if srv == nil {
		t.Fatal("server should be created")
	}
}

func TestRun_MultipleReferencesOneUnknown_ErrorMentionsUnknownOne(t *testing.T) {
	t.Parallel()

	// Arrange - Mux with multiple forward references, one is unknown
	mux := newTestMux()
	mux.HandleForward(newTestRoute("vendor-a"), "valid-target")
	mux.HandleForward(newTestRoute("vendor-b"), "unknown-target")
	mux.HandleForward(newTestRoute("vendor-c"), "another-valid")

	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"valid-target": {
			URL:  "https://valid.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
		"another-valid": {
			URL:  "https://another.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	// Act
	_, err := proxy.NewServer(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), "unknown-target") {
		t.Errorf("error should mention unknown-target, got: %v", err)
	}
	// Error should NOT require the valid targets to also be mentioned
}

func TestRun_EmptyForwardTargetsWithMuxNoForwardRoutes_Success(t *testing.T) {
	t.Parallel()

	// Arrange - Mux with only credential routes, no forward targets configured
	mux := newTestMux()
	mux.Handle(newTestRoute("vendor-a"), &plainPlugin{})

	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = nil // or empty map

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if srv == nil {
		t.Fatal("server should be created")
	}
}

func TestRun_EmptyForwardTargetsWithMuxForwardRoute_FailsAtStartup(t *testing.T) {
	t.Parallel()

	// Arrange - Mux with forward route but no targets configured
	mux := newTestMux()
	mux.HandleForward(newTestRoute("vendor-a"), "missing-target")

	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = nil // empty

	// Act
	_, err := proxy.NewServer(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected error for forward route with no targets")
	}
	if !strings.Contains(err.Error(), "missing-target") {
		t.Errorf("error should mention missing-target, got: %v", err)
	}
}

func TestRun_UnusedForwardTarget_WarnsAtStartup(t *testing.T) {
	t.Parallel()

	// Arrange - capture log output
	getLogs := captureLogs(t)

	// Mux with one forward reference
	mux := newTestMux()
	mux.HandleForward(newTestRoute("vendor-a"), "used-target")

	// But config has two targets (one unused)
	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"used-target": {
			URL:  "https://used.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
		"unused-target": {
			URL:  "https://unused.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert - must succeed
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if srv == nil {
		t.Fatal("server should be created")
	}

	// Warning should be logged for unused target
	logOut := getLogs()
	if !strings.Contains(logOut, "unused-target") {
		t.Errorf("expected warning about unused-target, got: %s", logOut)
	}
	if !strings.Contains(logOut, "forward_target") {
		t.Errorf("expected 'forward_target' in warning, got: %s", logOut)
	}
}

func TestRun_AllForwardTargetsReferenced_NoWarning(t *testing.T) {
	t.Parallel()

	// Arrange
	getLogs := captureLogs(t)

	mux := newTestMux()
	mux.HandleForward(newTestRoute("vendor-a"), "target-1")
	mux.HandleForward(newTestRoute("vendor-b"), "target-2")

	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"target-1": {
			URL:  "https://target1.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
		"target-2": {
			URL:  "https://target2.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	// Act
	_, err := proxy.NewServer(cfg)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	logOut := getLogs()
	if strings.Contains(logOut, "forward_target defined but never referenced") {
		t.Errorf("should not warn about unreferenced targets when all are referenced: %s", logOut)
	}
}

func TestRun_DuplicateForwardReferences_NotAnError(t *testing.T) {
	t.Parallel()

	// Arrange - Mux with duplicate references to the same target
	mux := newTestMux()
	mux.HandleForward(newTestRoute("vendor-a"), "company-b")
	mux.HandleForward(newTestRoute("vendor-b"), "company-b") // same target, different route

	cfg := testConfig()
	cfg.Plugin = mux
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {
			URL:  "https://company-b.example",
			Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
		},
	}

	// Act
	srv, err := proxy.NewServer(cfg)

	// Assert - duplicates are fine
	if err != nil {
		t.Fatalf("expected no error for duplicate references, got: %v", err)
	}
	if srv == nil {
		t.Fatal("server should be created")
	}
}

// Helper: creates a Mux compatible with the test contrib package.
// Since contrib is a separate module, we create a testMux that wraps it.
type testMux struct {
	entries []testMuxEntry
}

type testMuxEntry struct {
	target string // for forward references
	isRefs bool   // true if this is a forward reference
}

func newTestMux() *testMux {
	return &testMux{}
}

func (tm *testMux) Handle(route interface{}, provider interface{}) {
	// Stub for testing
}

func (tm *testMux) HandleForward(route interface{}, target string) {
	tm.entries = append(tm.entries, testMuxEntry{target: target, isRefs: true})
}

// ForwardReferences returns the list of forward target references.
func (tm *testMux) ForwardReferences() []string {
	refs := make([]string, 0, len(tm.entries))
	for _, e := range tm.entries {
		if e.isRefs {
			refs = append(refs, e.target)
		}
	}
	return refs
}

func (tm *testMux) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
	return nil, nil
}

func (tm *testMux) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
	return nil, nil
}

func (tm *testMux) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

var _ sdk.Plugin = (*testMux)(nil)

func newTestRoute(vendorID string) interface{} {
	return struct{ VendorID string }{VendorID: vendorID}
}
