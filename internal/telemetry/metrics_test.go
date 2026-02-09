// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// NOTE: These tests must NOT use t.Parallel() because they share global
// Prometheus metrics. Test isolation is achieved via Reset() and t.Cleanup().

func TestRequestsTotal_Increment(t *testing.T) {
	// Reset metrics for test isolation
	RequestsTotal.Reset()
	t.Cleanup(func() { RequestsTotal.Reset() })

	// Increment counter
	RequestsTotal.WithLabelValues("microsoft", "2xx", "POST").Inc()
	RequestsTotal.WithLabelValues("microsoft", "2xx", "POST").Inc()
	RequestsTotal.WithLabelValues("adobe", "5xx", "GET").Inc()

	// Verify counts
	count := testutil.ToFloat64(RequestsTotal.WithLabelValues("microsoft", "2xx", "POST"))
	if count != 2 {
		t.Errorf("expected count 2 for microsoft/2xx/POST, got %v", count)
	}

	count = testutil.ToFloat64(RequestsTotal.WithLabelValues("adobe", "5xx", "GET"))
	if count != 1 {
		t.Errorf("expected count 1 for adobe/5xx/GET, got %v", count)
	}
}

func TestRequestDuration_Observe(t *testing.T) {
	// Reset metrics for test isolation
	RequestDuration.Reset()
	t.Cleanup(func() { RequestDuration.Reset() })

	// Record some durations
	RequestDuration.WithLabelValues("microsoft").Observe(0.1)
	RequestDuration.WithLabelValues("microsoft").Observe(0.2)

	// Verify histogram has observations (use Collect to verify it works)
	ch := make(chan prometheus.Metric, 10)
	RequestDuration.Collect(ch)

	if len(ch) == 0 {
		t.Error("expected histogram to have observations")
	}
}

func TestActiveConnections_IncDec(t *testing.T) {
	// Reset gauge
	ActiveConnections.Set(0)
	t.Cleanup(func() { ActiveConnections.Set(0) })

	// Simulate connection lifecycle
	ActiveConnections.Inc()
	ActiveConnections.Inc()

	count := testutil.ToFloat64(ActiveConnections)
	if count != 2 {
		t.Errorf("expected 2 active connections, got %v", count)
	}

	ActiveConnections.Dec()
	count = testutil.ToFloat64(ActiveConnections)
	if count != 1 {
		t.Errorf("expected 1 active connection after dec, got %v", count)
	}
}

func TestStatusClass(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{299, "2xx"},
		{301, "3xx"},
		{304, "3xx"},
		{400, "4xx"},
		{404, "4xx"},
		{499, "4xx"},
		{500, "5xx"},
		{503, "5xx"},
		{599, "5xx"},
		{100, "other"},
		{600, "other"},
		{0, "other"},
	}

	for _, tt := range tests {
		t.Run(StatusString(tt.code), func(t *testing.T) {
			got := StatusClass(tt.code)
			if got != tt.expected {
				t.Errorf("StatusClass(%d) = %s, want %s", tt.code, got, tt.expected)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	// Test pre-allocated strings
	if StatusString(200) != "200" {
		t.Errorf("expected '200', got %s", StatusString(200))
	}
	if StatusString(404) != "404" {
		t.Errorf("expected '404', got %s", StatusString(404))
	}

	// Test fallback to strconv
	if StatusString(418) != "418" {
		t.Errorf("expected '418', got %s", StatusString(418))
	}
}

func TestNormalizeVendorID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", DefaultVendorID},
		{"normal", "microsoft", "microsoft"},
		{"max length", strings.Repeat("a", MaxVendorIDLength), strings.Repeat("a", MaxVendorIDLength)},
		{"too long", strings.Repeat("b", MaxVendorIDLength+10), strings.Repeat("b", MaxVendorIDLength)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeVendorID(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeVendorID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDefaultVendorID(t *testing.T) {
	if DefaultVendorID != "unknown" {
		t.Errorf("expected DefaultVendorID to be 'unknown', got %s", DefaultVendorID)
	}
}

func TestAPILatencyBuckets(t *testing.T) {
	// Verify buckets are reasonable for API latency
	if len(APILatencyBuckets) == 0 {
		t.Error("expected APILatencyBuckets to have values")
	}
	// First bucket should be around 10ms for fast APIs
	if APILatencyBuckets[0] != 0.01 {
		t.Errorf("expected first bucket to be 0.01 (10ms), got %v", APILatencyBuckets[0])
	}
	// Should have granularity in 100-500ms range
	found150ms := false
	for _, b := range APILatencyBuckets {
		if b == 0.15 {
			found150ms = true
			break
		}
	}
	if !found150ms {
		t.Error("expected APILatencyBuckets to have 0.15 (150ms) bucket for granularity")
	}
}
