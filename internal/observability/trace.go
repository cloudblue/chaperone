// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
)

// maxTraceIDLen is the maximum allowed length for an inbound trace ID.
// 256 bytes is generous — UUIDv4 is 36 chars, W3C traceparent is 55.
// IDs exceeding this are replaced with a generated UUIDv4 to prevent
// log bloat and unbounded header forwarding to vendor backends.
const maxTraceIDLen = 256

// traceIDKey is an unexported type for the context key that stores
// the trace ID, preventing collisions with keys from other packages.
type traceIDKey struct{}

// WithTraceID stores a trace ID in the context for propagation through
// the request lifecycle. All downstream handlers and the request logger
// can retrieve it via TraceIDFromContext.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFromContext retrieves the trace ID from the context.
// Returns an empty string if no trace ID has been stored.
func TraceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

// ExtractOrGenerateTraceID reads the trace ID from the configured request
// header. If the header is absent, empty, or fails validation, a new UUIDv4
// is generated.
//
// Validation (defense-in-depth):
//   - Length capped at maxTraceIDLen (256 bytes)
//   - Only printable ASCII allowed (alphanumeric, dashes, underscores,
//     dots, colons, slashes, equals, plus — covers UUID, W3C traceparent,
//     and base64 formats)
//
// Per Design Spec §8.3.1:
//   - Inbound: Extract the ID from the upstream client
//   - If missing (e.g., local testing): generate a UUIDv4
func ExtractOrGenerateTraceID(r *http.Request, traceHeader string) string {
	traceID := r.Header.Get(traceHeader)
	if traceID == "" {
		return generateUUIDv4()
	}
	if sanitized, ok := sanitizeTraceID(traceID); ok {
		return sanitized
	}
	generated := generateUUIDv4()
	slog.Warn("invalid trace ID from upstream, generated replacement",
		"header", traceHeader,
		"trace_id", generated,
		"original_len", len(traceID),
	)
	return generated
}

// sanitizeTraceID validates a trace ID for length and character safety.
// Returns the trace ID and true if valid, or empty string and false if not.
func sanitizeTraceID(id string) (string, bool) {
	if len(id) > maxTraceIDLen {
		return "", false
	}
	for i := 0; i < len(id); i++ {
		if !isValidTraceChar(id[i]) {
			return "", false
		}
	}
	return id, true
}

// isValidTraceChar returns true for printable ASCII characters commonly found
// in trace IDs: alphanumeric, dash, underscore, dot, colon, slash, equals,
// plus, and space.
func isValidTraceChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '-', c == '_', c == '.', c == ':', c == '/', c == '=', c == '+', c == ' ':
		return true
	default:
		return false
	}
}

// TraceIDMiddleware extracts or generates a trace ID early in the request
// lifecycle and stores it in the request context. It also ensures the trace
// header is set on the request so downstream services receive it.
//
// Per Design Spec §8.3.1:
//   - Propagation (Downstream): The ID is injected into the vendor request
//   - Propagation (Logs): Every log line includes "trace_id"
func TraceIDMiddleware(traceHeader string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := ExtractOrGenerateTraceID(r, traceHeader)

		// Store in context for downstream access (logging, plugin context)
		ctx := WithTraceID(r.Context(), traceID)

		// Ensure the header is set on the request for downstream propagation.
		// If the trace ID was generated (header was missing), this makes it
		// available to the reverse proxy Director which copies request headers
		// to the vendor request.
		r.Header.Set(traceHeader, traceID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateUUIDv4 generates a cryptographically random UUIDv4.
// Uses crypto/rand per security requirements (no timestamp-based IDs).
func generateUUIDv4() string {
	var uuid [16]byte
	_, err := rand.Read(uuid[:])
	if err != nil {
		// INTENTIONAL PANIC: This is a deliberate exception to the "never panic"
		// rule (go-errors.instructions.md). crypto/rand.Read failing means the
		// OS entropy source is broken — a non-recoverable condition that makes
		// secure operation impossible. Returning a fallback ID would silently
		// compromise trace uniqueness guarantees.
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	// Set version (4) and variant (RFC 4122)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
