// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"net/http"
	"time"

	"github.com/cloudblue/chaperone/internal/httputil"
)

// MetricsMiddleware instruments HTTP handlers with Prometheus metrics.
// It records request count, duration, and tracks active connections.
//
// Per Design Spec Section 8.3.2:
//   - chaperone_requests_total{vendor_id, status_class, method}
//   - chaperone_request_duration_seconds{vendor_id}
//   - chaperone_active_connections
//
// Note: This middleware extracts vendor_id directly from headers rather than
// using TransactionContext parsing because it runs before handleProxy.
// This is intentional - simple header lookup is sufficient for metrics.
func MetricsMiddleware(headerPrefix string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Track active connections
		ActiveConnections.Inc()
		defer ActiveConnections.Dec()

		start := time.Now()

		// Extract and normalize vendor ID from context headers
		// Note: Simple lookup is sufficient for metrics; full context parsing happens in handleProxy
		vendorID := NormalizeVendorID(r.Header.Get(headerPrefix + "-Vendor-ID"))

		// Add timing context for upstream duration tracking
		ctx, timing := WithTiming(r.Context())
		r = r.WithContext(ctx)

		// Wrap response writer to capture status code
		wrapped := httputil.NewStatusCapturingResponseWriter(w)

		// Process request
		next.ServeHTTP(wrapped, r)

		// Record metrics
		duration := time.Since(start).Seconds()

		RequestsTotal.WithLabelValues(
			vendorID,
			StatusClass(wrapped.Status),
			r.Method,
		).Inc()

		RequestDuration.WithLabelValues(vendorID).Observe(duration)

		// Record upstream duration if available
		if upstreamDur := timing.UpstreamDuration(); upstreamDur > 0 {
			UpstreamDuration.WithLabelValues(vendorID).Observe(upstreamDur.Seconds())
		}
	})
}
