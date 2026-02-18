// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Integration tests use fixed ports (19090-19094) to avoid conflicts with
// production services. These tests should NOT run with t.Parallel() to
// prevent port conflicts between test cases.

// httpGet performs an HTTP GET with a context timeout to prevent hangs.
func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	return resp
}

func TestAdminServer_Integration_StartShutdown(t *testing.T) {
	srv := NewAdminServer("127.0.0.1:19090", "1.0.0")

	err := srv.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify health endpoint
	resp := httpGet(t, "http://127.0.0.1:19090/_ops/health")
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("failed to shutdown: %v", err)
	}
}

func TestAdminServer_Integration_WithPprof(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	srv := NewAdminServer("127.0.0.1:19091", "1.0.0")
	RegisterPprofHandlers(srv.Mux(), true)

	err := srv.Start()
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer srv.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Verify pprof is accessible
	resp := httpGet(t, "http://127.0.0.1:19091/debug/pprof/")
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected pprof status 200, got %d", resp.StatusCode)
	}
}

func TestAdminServer_Integration_HeapProfile(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	srv := NewAdminServer("127.0.0.1:19092", "1.0.0")
	RegisterPprofHandlers(srv.Mux(), true)

	err := srv.Start()
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer srv.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	resp := httpGet(t, "http://127.0.0.1:19092/debug/pprof/heap")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestAdminServer_Integration_PprofDisabled(t *testing.T) {
	srv := NewAdminServer("127.0.0.1:19093", "1.0.0")
	// Note: NOT registering pprof handlers

	err := srv.Start()
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer srv.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Health should work
	resp := httpGet(t, "http://127.0.0.1:19093/_ops/health")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected health status 200, got %d", resp.StatusCode)
	}

	// pprof should 404
	resp = httpGet(t, "http://127.0.0.1:19093/debug/pprof/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected pprof status 404, got %d", resp.StatusCode)
	}
}

func TestAdminServer_Integration_GoroutineProfile(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	srv := NewAdminServer("127.0.0.1:19094", "1.0.0")
	RegisterPprofHandlers(srv.Mux(), true)

	err := srv.Start()
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer srv.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	resp := httpGet(t, "http://127.0.0.1:19094/debug/pprof/goroutine")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// Compile-time check that fmt is used (for future table-driven tests)
var _ = fmt.Sprintf
