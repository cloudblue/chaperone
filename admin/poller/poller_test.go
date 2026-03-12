// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package poller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/admin/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open(%q) failed: %v", dbPath, err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func fakeProxy(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/_ops/health":
			w.Write([]byte(`{"status":"alive"}`))
		case "/_ops/version":
			w.Write([]byte(`{"version":"1.0.0"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProbe_HealthyProxy_ReturnsOK(t *testing.T) {
	t.Parallel()
	proxy := fakeProxy(t)
	addr := strings.TrimPrefix(proxy.URL, "http://")

	result := Probe(context.Background(), &http.Client{Timeout: 2 * time.Second}, addr)

	if !result.OK {
		t.Fatalf("expected OK=true, got error: %s", result.Error)
	}
	if result.Health != "alive" {
		t.Errorf("Health = %q, want %q", result.Health, "alive")
	}
	if result.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", result.Version, "1.0.0")
	}
}

func TestProbe_UnreachableAddress_ReturnsError(t *testing.T) {
	t.Parallel()

	result := Probe(context.Background(), &http.Client{Timeout: 1 * time.Second}, "127.0.0.1:1")

	if result.OK {
		t.Error("expected OK=false for unreachable address")
	}
	if result.Error == "" {
		t.Error("expected non-empty error")
	}
}

func TestProbe_HealthEndpointError_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	result := Probe(context.Background(), &http.Client{Timeout: 2 * time.Second}, addr)

	if result.OK {
		t.Error("expected OK=false for error status")
	}
}

func TestPoller_SinglePoll_SetsHealthy(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	proxy := fakeProxy(t)
	addr := strings.TrimPrefix(proxy.URL, "http://")

	ctx := context.Background()
	inst, err := st.CreateInstance(ctx, "test-proxy", addr)
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	p := New(st, 1*time.Hour, 2*time.Second) // long interval; we call pollAll manually.
	p.pollAll(ctx)

	got, err := st.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if got.Status != "healthy" {
		t.Errorf("Status = %q, want %q", got.Status, "healthy")
	}
	if got.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", got.Version, "1.0.0")
	}
}

func TestPoller_ThreeFailures_SetsUnreachable(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	ctx := context.Background()
	inst, err := st.CreateInstance(ctx, "test-proxy", "127.0.0.1:1")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	p := New(st, 1*time.Hour, 500*time.Millisecond)

	// Poll 3 times to reach unreachable threshold.
	for i := 0; i < failuresUntilUnreachable; i++ {
		p.pollAll(ctx)
	}

	got, err := st.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if got.Status != "unreachable" {
		t.Errorf("Status = %q, want %q after %d failures", got.Status, "unreachable", failuresUntilUnreachable)
	}
}

func TestPoller_TwoFailures_StaysUnknown(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	ctx := context.Background()
	inst, err := st.CreateInstance(ctx, "test-proxy", "127.0.0.1:1")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	p := New(st, 1*time.Hour, 500*time.Millisecond)

	// Poll only twice — should not yet transition to unreachable.
	for i := 0; i < failuresUntilUnreachable-1; i++ {
		p.pollAll(ctx)
	}

	got, err := st.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if got.Status != "unknown" {
		t.Errorf("Status = %q, want %q after %d failures", got.Status, "unknown", failuresUntilUnreachable-1)
	}
}

func TestPoller_RecoveryAfterUnreachable_SetsHealthy(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	proxy := fakeProxy(t)
	addr := strings.TrimPrefix(proxy.URL, "http://")

	ctx := context.Background()
	inst, err := st.CreateInstance(ctx, "test-proxy", "127.0.0.1:1")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	p := New(st, 1*time.Hour, 500*time.Millisecond)

	// Drive to unreachable.
	for i := 0; i < failuresUntilUnreachable; i++ {
		p.pollAll(ctx)
	}

	// Now point instance to the live proxy.
	_, updateErr := st.UpdateInstance(ctx, inst.ID, "test-proxy", addr)
	if updateErr != nil {
		t.Fatalf("UpdateInstance() error = %v", updateErr)
	}

	// Single success should recover.
	p.pollAll(ctx)

	got, err := st.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if got.Status != "healthy" {
		t.Errorf("Status = %q, want %q after recovery", got.Status, "healthy")
	}
}

func TestPoller_DeletedInstance_PrunesFailures(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	ctx := context.Background()
	inst, err := st.CreateInstance(ctx, "test-proxy", "127.0.0.1:1")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	p := New(st, 1*time.Hour, 500*time.Millisecond)

	// Accumulate failures.
	p.pollAll(ctx)

	p.mu.Lock()
	count := p.failures[inst.ID]
	p.mu.Unlock()
	if count != 1 {
		t.Fatalf("failures[%d] = %d, want 1", inst.ID, count)
	}

	// Delete the instance and poll again.
	if err := st.DeleteInstance(ctx, inst.ID); err != nil {
		t.Fatalf("DeleteInstance() error = %v", err)
	}
	p.pollAll(ctx)

	p.mu.Lock()
	_, exists := p.failures[inst.ID]
	p.mu.Unlock()
	if exists {
		t.Errorf("failures[%d] still present after instance deletion", inst.ID)
	}
}

func TestPoller_RunStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	p := New(st, 50*time.Millisecond, 500*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// OK — Run returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}
