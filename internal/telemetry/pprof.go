// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"log/slog"
	"net/http"
	"net/http/pprof"
)

// allowProfiling controls whether pprof endpoints can be enabled.
// This is set at compile time via ldflags. Default is "false" (disabled).
//
// SECURITY: In production builds, this MUST be "false" to prevent
// exposure of sensitive runtime information (heap dumps, goroutine stacks).
//
// Set via: -ldflags "-X 'github.com/cloudblue/chaperone/internal/telemetry.allowProfiling=true'"
var allowProfiling = "false"

// AllowProfiling returns true if pprof can be enabled (dev builds only).
func AllowProfiling() bool {
	return allowProfiling == "true"
}

// RegisterPprofHandlers registers pprof endpoints on the given mux.
// Returns false if profiling is not allowed (production builds) or not enabled.
//
// Registered endpoints:
//   - /debug/pprof/           - Index page with links to all profiles
//   - /debug/pprof/cmdline    - Command line arguments
//   - /debug/pprof/profile    - CPU profile (add ?seconds=N)
//   - /debug/pprof/symbol     - Symbol lookup
//   - /debug/pprof/trace      - Execution trace
//   - /debug/pprof/heap       - Heap profile (via Index)
//   - /debug/pprof/goroutine  - Goroutine stack dump (via Index)
//   - /debug/pprof/block      - Block profile (via Index)
//   - /debug/pprof/mutex      - Mutex contention profile (via Index)
//   - /debug/pprof/allocs     - Allocation profile (via Index)
func RegisterPprofHandlers(mux *http.ServeMux, enabled bool) bool {
	if !AllowProfiling() {
		slog.Debug("pprof disabled at build time (production build)")
		return false
	}

	if !enabled {
		slog.Debug("pprof not enabled via configuration")
		return false
	}

	slog.Warn("pprof endpoints enabled - FOR DEVELOPMENT/DEBUGGING ONLY")

	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	return true
}
