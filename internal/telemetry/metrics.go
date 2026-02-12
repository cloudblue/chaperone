// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// APILatencyBuckets are histogram buckets optimized for typical API response times.
// Default Prometheus buckets (.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10) are
// suboptimal for API proxies where most requests fall in the 50-500ms range.
//
// Buckets provide granularity in the critical 100-500ms range for accurate percentiles.
// Observations > 10s will fall into the +Inf bucket.
var APILatencyBuckets = []float64{
	.01, .025, .05, .1, .15, .2, .25, .3, .4, .5, .75, 1, 2, 5, 10,
}

// Metrics for the Chaperone proxy.
// Per Design Spec Section 8.3.2: Metrics (Performance Telemetry)
//
// Label cardinality notes:
//   - vendor_id: Expected low cardinality (tens of vendors), validated against [a-zA-Z0-9._-] and truncated to 64 chars
//   - status_class: Bucketed to 2xx/3xx/4xx/5xx/other (5 values max)
//   - method: Limited HTTP methods (< 10 values)
//   - AVOID: subscription_id, product_id (high cardinality)
//
// Registry note:
//
//	Metrics are registered with prometheus.DefaultRegistry via promauto.
//	This simplifies the code and matches the expected operational pattern
//	where a single process serves metrics. For test isolation, use Reset()
//	on metric vectors with t.Cleanup().
//
// Thread-safety note:
//
//	These tests must NOT use t.Parallel() because they share global
//	Prometheus metrics. Test isolation is achieved via Reset() and t.Cleanup().
var (
	// RequestsTotal counts total requests processed.
	// Labels: vendor_id, status_class, method
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "chaperone",
			Name:      "requests_total",
			Help:      "Total number of requests processed",
		},
		[]string{"vendor_id", "status_class", "method"},
	)

	// RequestDuration measures total request duration (including plugin and upstream).
	// Labels: vendor_id
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "chaperone",
			Name:      "request_duration_seconds",
			Help:      "Total request duration including plugin and upstream",
			Buckets:   APILatencyBuckets,
		},
		[]string{"vendor_id"},
	)

	// UpstreamDuration measures time spent waiting for upstream response.
	// Labels: vendor_id
	UpstreamDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "chaperone",
			Name:      "upstream_duration_seconds",
			Help:      "Time spent waiting for upstream response",
			Buckets:   APILatencyBuckets,
		},
		[]string{"vendor_id"},
	)

	// ActiveConnections tracks number of active connections (in-flight requests).
	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "chaperone",
			Name:      "active_connections",
			Help:      "Number of active connections",
		},
	)

	// PanicsTotal counts total recovered panics.
	// Exposes the panic count from WithPanicRecovery middleware as a Prometheus metric.
	PanicsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "chaperone",
			Name:      "panics_total",
			Help:      "Total number of recovered panics",
		},
	)
)

// DefaultVendorID is the label value used when the X-Connect-Vendor-ID header
// is missing from a request.
const DefaultVendorID = "unknown"

// MaxVendorIDLength is the maximum length for vendor_id label to prevent
// unbounded cardinality from malicious or misconfigured clients.
const MaxVendorIDLength = 64

// statusStrings pre-allocates common status code strings to avoid allocations.
// Used by StatusString for logging, debugging, and test naming.
// Note: Metrics use StatusClass (2xx/4xx/5xx) for label cardinality control.
var statusStrings = map[int]string{
	200: "200", 201: "201", 202: "202", 204: "204",
	301: "301", 302: "302", 304: "304",
	400: "400", 401: "401", 403: "403", 404: "404", 405: "405", 408: "408", 429: "429",
	500: "500", 501: "501", 502: "502", 503: "503", 504: "504",
}

// StatusClass returns a bucketed status class (2xx, 3xx, 4xx, 5xx, other)
// to reduce label cardinality while preserving useful information.
func StatusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "other"
	}
}

// StatusString returns a pre-allocated string for common status codes,
// falling back to strconv.Itoa for uncommon codes.
// Used for logging, debugging, and test naming (not for metric labels - use StatusClass).
func StatusString(code int) string {
	if s, ok := statusStrings[code]; ok {
		return s
	}
	return strconv.Itoa(code)
}

// NormalizeVendorID ensures vendor_id is safe for use as a metric label.
// It returns DefaultVendorID for empty strings or strings containing characters
// outside the allowed set [a-zA-Z0-9._-]. Long values are truncated to
// MaxVendorIDLength.
//
// This prevents unbounded label cardinality from malicious or misconfigured
// clients sending arbitrary strings in the X-Connect-Vendor-ID header.
func NormalizeVendorID(id string) string {
	if id == "" {
		return DefaultVendorID
	}
	if len(id) > MaxVendorIDLength {
		id = id[:MaxVendorIDLength]
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '.' && c != '_' && c != '-' {
			return DefaultVendorID
		}
	}
	return id
}
