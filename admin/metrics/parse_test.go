// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"testing"
	"time"
)

const sampleMetrics = `# HELP chaperone_requests_total Total number of requests processed
# TYPE chaperone_requests_total counter
chaperone_requests_total{vendor_id="acme",status_class="2xx",method="GET"} 1500
chaperone_requests_total{vendor_id="acme",status_class="2xx",method="POST"} 300
chaperone_requests_total{vendor_id="acme",status_class="4xx",method="GET"} 50
chaperone_requests_total{vendor_id="acme",status_class="5xx",method="POST"} 5
chaperone_requests_total{vendor_id="beta",status_class="2xx",method="GET"} 800
chaperone_requests_total{vendor_id="beta",status_class="4xx",method="GET"} 20
# HELP chaperone_panics_total Total number of recovered panics
# TYPE chaperone_panics_total counter
chaperone_panics_total 2
# HELP chaperone_request_duration_seconds Total request duration
# TYPE chaperone_request_duration_seconds histogram
chaperone_request_duration_seconds_bucket{vendor_id="acme",le="0.05"} 600
chaperone_request_duration_seconds_bucket{vendor_id="acme",le="0.1"} 1000
chaperone_request_duration_seconds_bucket{vendor_id="acme",le="0.25"} 1500
chaperone_request_duration_seconds_bucket{vendor_id="acme",le="0.5"} 1700
chaperone_request_duration_seconds_bucket{vendor_id="acme",le="1"} 1800
chaperone_request_duration_seconds_bucket{vendor_id="acme",le="+Inf"} 1855
chaperone_request_duration_seconds_sum{vendor_id="acme"} 250.5
chaperone_request_duration_seconds_count{vendor_id="acme"} 1855
chaperone_request_duration_seconds_bucket{vendor_id="beta",le="0.05"} 400
chaperone_request_duration_seconds_bucket{vendor_id="beta",le="0.1"} 700
chaperone_request_duration_seconds_bucket{vendor_id="beta",le="0.25"} 780
chaperone_request_duration_seconds_bucket{vendor_id="beta",le="0.5"} 800
chaperone_request_duration_seconds_bucket{vendor_id="beta",le="1"} 810
chaperone_request_duration_seconds_bucket{vendor_id="beta",le="+Inf"} 820
chaperone_request_duration_seconds_sum{vendor_id="beta"} 80.0
chaperone_request_duration_seconds_count{vendor_id="beta"} 820
# HELP chaperone_active_connections Number of active connections
# TYPE chaperone_active_connections gauge
chaperone_active_connections 15
`

func TestParse_FullSample_ExtractsAllMetrics(t *testing.T) {
	t.Parallel()
	now := time.Now()

	snap, err := Parse([]byte(sampleMetrics), now)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if snap.Time != now {
		t.Errorf("Time = %v, want %v", snap.Time, now)
	}
	if snap.ActiveConnections != 15 {
		t.Errorf("ActiveConnections = %v, want 15", snap.ActiveConnections)
	}
	if snap.PanicsTotal != 2 {
		t.Errorf("PanicsTotal = %v, want 2", snap.PanicsTotal)
	}
	if len(snap.Vendors) != 2 {
		t.Fatalf("Vendors count = %d, want 2", len(snap.Vendors))
	}
}

func TestParse_Requests_SumsAcrossMethods(t *testing.T) {
	t.Parallel()

	snap, err := Parse([]byte(sampleMetrics), time.Now())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	acme := snap.Vendors["acme"]
	if acme == nil {
		t.Fatal("missing vendor 'acme'")
	}
	// GET 1500 + POST 300 (2xx) + GET 50 (4xx) + POST 5 (5xx) = 1855
	if acme.RequestsTotal != 1855 {
		t.Errorf("acme.RequestsTotal = %v, want 1855", acme.RequestsTotal)
	}
	// 4xx: 50 + 5xx: 5 = 55
	if acme.RequestsErrors != 55 {
		t.Errorf("acme.RequestsErrors = %v, want 55", acme.RequestsErrors)
	}

	beta := snap.Vendors["beta"]
	if beta == nil {
		t.Fatal("missing vendor 'beta'")
	}
	// GET 800 (2xx) + GET 20 (4xx) = 820
	if beta.RequestsTotal != 820 {
		t.Errorf("beta.RequestsTotal = %v, want 820", beta.RequestsTotal)
	}
	if beta.RequestsErrors != 20 {
		t.Errorf("beta.RequestsErrors = %v, want 20", beta.RequestsErrors)
	}
}

func TestParse_Histogram_ParsesBucketsCorrectly(t *testing.T) {
	t.Parallel()

	snap, err := Parse([]byte(sampleMetrics), time.Now())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	acme := snap.Vendors["acme"]
	h := acme.Duration
	if h.Count != 1855 {
		t.Errorf("acme histogram Count = %v, want 1855", h.Count)
	}
	if h.Sum != 250.5 {
		t.Errorf("acme histogram Sum = %v, want 250.5", h.Sum)
	}
	// +Inf bucket should be stripped
	if len(h.Buckets) != 5 {
		t.Fatalf("acme histogram Buckets = %d, want 5", len(h.Buckets))
	}
	if h.Buckets[0].UpperBound != 0.05 {
		t.Errorf("first bucket UpperBound = %v, want 0.05", h.Buckets[0].UpperBound)
	}
	if h.Buckets[0].Count != 600 {
		t.Errorf("first bucket Count = %v, want 600", h.Buckets[0].Count)
	}
}

func TestParse_EmptyInput_ReturnsEmptySnapshot(t *testing.T) {
	t.Parallel()

	snap, err := Parse([]byte(""), time.Now())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(snap.Vendors) != 0 {
		t.Errorf("Vendors count = %d, want 0", len(snap.Vendors))
	}
	if snap.ActiveConnections != 0 {
		t.Errorf("ActiveConnections = %v, want 0", snap.ActiveConnections)
	}
}

func TestParse_UnknownMetrics_Ignored(t *testing.T) {
	t.Parallel()
	input := `# HELP custom_metric A custom metric
# TYPE custom_metric gauge
custom_metric 42
# HELP chaperone_active_connections Number of active connections
# TYPE chaperone_active_connections gauge
chaperone_active_connections 7
`
	snap, err := Parse([]byte(input), time.Now())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if snap.ActiveConnections != 7 {
		t.Errorf("ActiveConnections = %v, want 7", snap.ActiveConnections)
	}
}

func TestParse_MalformedInput_ReturnsError(t *testing.T) {
	t.Parallel()
	// Invalid Prometheus format: duplicate TYPE declaration
	input := `# TYPE chaperone_active_connections gauge
# TYPE chaperone_active_connections counter
chaperone_active_connections 7
`
	_, err := Parse([]byte(input), time.Now())
	if err == nil {
		t.Error("expected error for malformed input, got nil")
	}
}
