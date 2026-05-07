// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ResponseCapturer wraps http.ResponseWriter to capture the status code
// written by downstream handlers. This is needed for request logging
// since the standard ResponseWriter doesn't expose the status code.
type ResponseCapturer struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// NewResponseCapturer creates a ResponseCapturer wrapping the given writer.
// The default status is 200 (implicit Go behavior when WriteHeader is never called).
func NewResponseCapturer(w http.ResponseWriter) *ResponseCapturer {
	return &ResponseCapturer{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

// WriteHeader captures the status code and delegates to the inner writer.
// 1xx informational responses (e.g., 100 Continue) are forwarded without
// setting the guard, since httputil.ReverseProxy may send them before the
// final response. Only the first final (2xx+) call is forwarded; subsequent
// calls are silently ignored to avoid Go's "superfluous response.WriteHeader
// call" warning.
func (rc *ResponseCapturer) WriteHeader(code int) {
	// Pass through 1xx informational responses without setting the guard.
	if code >= 100 && code <= 199 {
		rc.ResponseWriter.WriteHeader(code)
		return
	}
	if rc.wroteHeader {
		return
	}
	rc.status = code
	rc.wroteHeader = true
	rc.ResponseWriter.WriteHeader(code)
}

// Write delegates to the inner writer. If WriteHeader hasn't been called,
// the status defaults to 200 (matching net/http behavior).
func (rc *ResponseCapturer) Write(b []byte) (int, error) {
	if !rc.wroteHeader {
		rc.wroteHeader = true
		// status stays at default 200
	}
	return rc.ResponseWriter.Write(b)
}

// Flush implements http.Flusher to support streaming responses.
// Some vendors (e.g., LLM/AI services) may stream responses.
func (rc *ResponseCapturer) Flush() {
	if f, ok := rc.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter.
// This supports Go 1.20+ http.ResponseController, which calls Unwrap()
// to discover optional interfaces (e.g., SetWriteDeadline) on the inner writer.
func (rc *ResponseCapturer) Unwrap() http.ResponseWriter {
	return rc.ResponseWriter
}

// Status returns the captured HTTP status code.
func (rc *ResponseCapturer) Status() int {
	return rc.status
}

// RequestLoggerMiddleware logs every completed request with structured fields
// per Design Spec §8.3.5.
//
// Parameters:
//   - logger: structured logger for output
//   - headerPrefix: the context header prefix (e.g., "X-Connect"). The middleware
//     constructs header names internally using the stable suffix constants.
//   - next: the downstream handler
//
// Fields emitted:
//   - trace_id: Correlation ID from context (set by TraceIDMiddleware)
//   - method: HTTP method
//   - path: Request path
//   - status: Response status code
//   - latency_ms: Time to process request in milliseconds
//   - vendor_id: Vendor ID from the <prefix>-Vendor-ID request header
//   - marketplace_id: Marketplace ID from the <prefix>-Marketplace-ID request header
//   - product_id: Product ID from the <prefix>-Product-ID request header
//   - target_host: Host extracted from the <prefix>-Target-URL request header
//   - client_ip: Client IP from proxy headers (X-Forwarded-For > X-Real-IP);
//     empty when no proxy headers are present (use remote_addr instead)
//   - remote_addr: Raw TCP peer address (always r.RemoteAddr, useful for
//     debugging network topology and correlating across L4 LB hops)
//
// This middleware must run INSIDE TraceIDMiddleware so the trace ID is
// already in the request context when this handler receives the request.
// Uses defer to ensure logging occurs even if downstream handlers panic
// (when used with panic recovery middleware).
func RequestLoggerMiddleware(logger *slog.Logger, headerPrefix string, next http.Handler) http.Handler {
	vendorHdr := headerPrefix + "-Vendor-ID"
	marketplaceHdr := headerPrefix + "-Marketplace-ID"
	productHdr := headerPrefix + "-Product-ID"
	targetURLHdr := headerPrefix + "-Target-URL"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := r.Context()

		// Wrap response writer to capture status code
		capturer := NewResponseCapturer(w)

		// Always log, even on panic (defer runs after recovery middleware)
		defer func() {
			logger.InfoContext(ctx, "request completed",
				"trace_id", TraceIDFromContext(ctx),
				"method", r.Method,
				"path", r.URL.Path,
				"status", capturer.Status(),
				"latency_ms", time.Since(start).Milliseconds(),
				"vendor_id", r.Header.Get(vendorHdr),
				"marketplace_id", r.Header.Get(marketplaceHdr),
				"product_id", r.Header.Get(productHdr),
				"target_host", extractHost(r.Header.Get(targetURLHdr)),
				"client_ip", ClientIP(r),
				"remote_addr", r.RemoteAddr,
			)
		}()

		next.ServeHTTP(capturer, r)
	})
}

// extractHost parses rawURL and returns only the host (with port if present).
// Returns an empty string if the URL is invalid or has no host.
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// ClientIP extracts the client IP from proxy headers only.
// Returns the first IP from X-Forwarded-For, or X-Real-IP as fallback.
// Returns "" when no proxy headers are present — in that case the
// separately logged remote_addr field is the client address.
func ClientIP(r *http.Request) string {
	// X-Forwarded-For: client, proxy1, proxy2
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(ip)
		}
		return strings.TrimSpace(xff)
	}

	// X-Real-IP: single IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	return ""
}
