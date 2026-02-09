// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAdminServer_CustomAddr(t *testing.T) {
	srv := NewAdminServer("127.0.0.1:8080")
	if srv.Addr() != "127.0.0.1:8080" {
		t.Errorf("expected custom addr 127.0.0.1:8080, got %s", srv.Addr())
	}
}

func TestAdminServer_HealthEndpoint(t *testing.T) {
	srv := NewAdminServer(":9090")

	req := httptest.NewRequest(http.MethodGet, "/_ops/health", nil)
	w := httptest.NewRecorder()

	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "alive") {
		t.Errorf("expected body to contain 'alive', got %s", w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

func TestAdminServer_Mux(t *testing.T) {
	srv := NewAdminServer(":9090")

	// Verify we can get the mux and register handlers
	mux := srv.Mux()
	if mux == nil {
		t.Error("expected non-nil mux")
	}

	// Register a custom handler
	mux.HandleFunc("GET /custom", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("custom"))
	})

	// Verify custom handler works
	req := httptest.NewRequest(http.MethodGet, "/custom", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for custom handler, got %d", w.Code)
	}
}

func TestAdminServer_Shutdown_Nil(t *testing.T) {
	// Test that Shutdown handles nil server gracefully
	var srv *AdminServer
	err := srv.Shutdown(nil)
	if err != nil {
		t.Errorf("expected nil error for nil server shutdown, got %v", err)
	}
}

func TestAdminServer_Shutdown_NotStarted(t *testing.T) {
	srv := NewAdminServer(":9090")
	// Server not started, should handle gracefully
	err := srv.Shutdown(nil)
	if err != nil {
		t.Errorf("expected nil error for non-started server shutdown, got %v", err)
	}
}

func TestAdminServer_MetricsEndpoint(t *testing.T) {
	srv := NewAdminServer("")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify Prometheus format (should contain go_* metrics at minimum)
	body := w.Body.String()
	if !strings.Contains(body, "go_goroutines") {
		t.Error("expected metrics output to contain Go runtime metrics")
	}
}

func TestAdminServer_MetricsContentType(t *testing.T) {
	srv := NewAdminServer("")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	srv.Mux().ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	// Prometheus handler returns text/plain for exposition format
	// or application/openmetrics-text for OpenMetrics format depending on Accept header
	if !strings.HasPrefix(contentType, "text/plain") &&
		!strings.HasPrefix(contentType, "application/openmetrics-text") {
		t.Errorf("expected Prometheus-compatible Content-Type, got %s", contentType)
	}
}
