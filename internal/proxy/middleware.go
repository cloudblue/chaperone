// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// panicCount tracks the total number of recovered panics.
var panicCount atomic.Int64

// WithPanicRecovery wraps a handler with panic recovery middleware.
// If a panic occurs, it logs the stack trace and returns a generic 500
// Internal Server Error as JSON (no internal details are exposed to the client).
// The server continues running after recovering from the panic.
func WithPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				panicCount.Add(1)

				// Log the panic with stack trace (internal only, never expose to client)
				slog.Error("panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
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

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher to support streaming responses.
// While the current Design Spec focuses on request/response APIs, some vendors
// (e.g., LLM/AI services) may stream responses. This preserves that capability.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// WithRequestLogging wraps a handler with request/response logging middleware.
// Per Design Spec Section 8.3.5, every request emits a log line including:
// trace_id, latency, status, method, path, and client_ip.
//
// The traceHeader parameter specifies which header contains the correlation ID
// (configured via upstream.trace_header, per ADR-005).
//
// Note: Sensitive headers are NOT logged here (redaction handled elsewhere).
// Uses defer to ensure logging happens even if downstream handlers panic.
func WithRequestLogging(traceHeader string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract trace ID from configured header
		traceID := r.Header.Get(traceHeader)

		// Wrap response writer to capture status
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		// Always log the request, even if handler panics
		defer func() {
			slog.Info("request completed",
				"trace_id", traceID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"latency_ms", time.Since(start).Milliseconds(),
				"client_ip", r.RemoteAddr,
			)
		}()

		// Process request
		next.ServeHTTP(wrapped, r)
	})
}
