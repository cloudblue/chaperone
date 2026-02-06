// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// SetAllowProfilingForTesting allows tests to override the build-time setting.
// Returns a cleanup function that restores the original value.
//
// WARNING: This function is NOT thread-safe. Do not use t.Parallel() in tests
// that call this helper, as concurrent modifications to the package-level
// variable will cause race conditions.
func SetAllowProfilingForTesting(allow bool) func() {
	origAllowProfiling := allowProfiling
	if allow {
		allowProfiling = "true"
	} else {
		allowProfiling = "false"
	}

	return func() {
		allowProfiling = origAllowProfiling
	}
}

func TestAllowProfiling_Default(t *testing.T) {
	// By default (without ldflags), profiling should be disabled
	cleanup := SetAllowProfilingForTesting(false)
	defer cleanup()

	if AllowProfiling() {
		t.Error("expected AllowProfiling to return false by default")
	}
}

func TestAllowProfiling_Enabled(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	if !AllowProfiling() {
		t.Error("expected AllowProfiling to return true when enabled")
	}
}

func TestRegisterPprofHandlers_DisabledAtBuildTime(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(false)
	defer cleanup()

	mux := http.NewServeMux()
	registered := RegisterPprofHandlers(mux, true)

	if registered {
		t.Error("expected false when profiling disabled at build time")
	}
}

func TestRegisterPprofHandlers_NotEnabled(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	mux := http.NewServeMux()
	registered := RegisterPprofHandlers(mux, false)

	if registered {
		t.Error("expected false when profiling not enabled via config")
	}
}

func TestRegisterPprofHandlers_Enabled(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	mux := http.NewServeMux()
	registered := RegisterPprofHandlers(mux, true)

	if !registered {
		t.Error("expected true when profiling enabled")
	}
}

func TestRegisterPprofHandlers_Endpoints(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	mux := http.NewServeMux()
	RegisterPprofHandlers(mux, true)

	testCases := []struct {
		name   string
		path   string
		method string
		status int
	}{
		{"index page", "/debug/pprof/", http.MethodGet, http.StatusOK},
		{"cmdline", "/debug/pprof/cmdline", http.MethodGet, http.StatusOK},
		{"heap profile", "/debug/pprof/heap", http.MethodGet, http.StatusOK},
		{"goroutine", "/debug/pprof/goroutine", http.MethodGet, http.StatusOK},
		{"allocs", "/debug/pprof/allocs", http.MethodGet, http.StatusOK},
		{"block", "/debug/pprof/block", http.MethodGet, http.StatusOK},
		{"mutex", "/debug/pprof/mutex", http.MethodGet, http.StatusOK},
		{"symbol GET", "/debug/pprof/symbol", http.MethodGet, http.StatusOK},
		{"symbol POST", "/debug/pprof/symbol", http.MethodPost, http.StatusOK},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tc.status {
				t.Errorf("expected status %d for %s %s, got %d",
					tc.status, tc.method, tc.path, w.Code)
			}
		})
	}
}

func TestRegisterPprofHandlers_IndexContent(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	mux := http.NewServeMux()
	RegisterPprofHandlers(mux, true)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	body := w.Body.String()
	expectedProfiles := []string{"heap", "goroutine", "block", "profile"}

	for _, profile := range expectedProfiles {
		if !strings.Contains(body, profile) {
			t.Errorf("pprof index should contain '%s'", profile)
		}
	}
}

func TestRegisterPprofHandlers_PprofNotRegistered_Returns404(t *testing.T) {
	cleanup := SetAllowProfilingForTesting(true)
	defer cleanup()

	mux := http.NewServeMux()
	// Note: NOT calling RegisterPprofHandlers

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 when pprof not registered, got %d", w.Code)
	}
}
