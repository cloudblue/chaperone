// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"

	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/internal/telemetry"
)

// panicCount tracks the total number of recovered panics.
var panicCount atomic.Int64

// PanicCount returns the total number of panics recovered since process start.
// This is exposed for the metrics/telemetry layer (Task 07).
func PanicCount() int64 {
	return panicCount.Load()
}

// PanicRecoveryMiddleware wraps a handler with panic recovery.
// If a panic occurs, it logs the stack trace and returns a generic 500
// Internal Server Error as JSON (no internal details are exposed to the client).
// The server continues running after recovering from the panic.
//
// When placed inside TraceIDMiddleware (the production ordering), the trace ID
// is available in the request context for inclusion in the panic log.
func PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		defer func() {
			if err := recover(); err != nil {
				panicCount.Add(1)
				telemetry.PanicsTotal.Inc()

				// Log the panic with stack trace (internal only, never expose to client)
				slog.Error("panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
					"trace_id", observability.TraceIDFromContext(ctx),
					"path", r.URL.Path,
					"method", r.Method,
				)

				// Return generic JSON error to client
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":  "Internal Server Error",
					"status": http.StatusInternalServerError,
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
