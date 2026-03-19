// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package metrics parses Prometheus metrics from Chaperone proxy instances,
// stores them in ring buffers, and computes rates and percentiles.
package metrics

import "time"

// DefaultCapacity is the number of scrape snapshots retained per instance.
// At 10s intervals this gives ~1 hour of history.
const DefaultCapacity = 360

// Prometheus metric names emitted by the Chaperone proxy.
const (
	metricRequestsTotal   = "chaperone_requests_total"
	metricDurationSeconds = "chaperone_request_duration_seconds"
	metricActiveConns     = "chaperone_active_connections"
	metricPanicsTotal     = "chaperone_panics_total"

	labelVendorID    = "vendor_id"
	labelStatusClass = "status_class"
)

// Snapshot holds parsed metrics from a single scrape of one proxy instance.
type Snapshot struct {
	Time              time.Time
	Vendors           map[string]*VendorSnapshot
	ActiveConnections float64
	PanicsTotal       float64
}

// VendorSnapshot holds per-vendor counters and histogram for a single scrape.
type VendorSnapshot struct {
	RequestsTotal  float64
	RequestsErrors float64 // 4xx + 5xx
	Duration       Histogram
}

// Histogram holds cumulative histogram bucket data.
type Histogram struct {
	Buckets []Bucket
	Count   float64
	Sum     float64
}

// Bucket is a single cumulative histogram bucket.
type Bucket struct {
	UpperBound float64
	Count      float64 // cumulative count of observations <= UpperBound
}

// vendorOrCreate returns the VendorSnapshot for the given vendor, creating it if needed.
func (s *Snapshot) vendorOrCreate(id string) *VendorSnapshot {
	if id == "" {
		id = "unknown"
	}
	vs, ok := s.Vendors[id]
	if !ok {
		vs = &VendorSnapshot{}
		s.Vendors[id] = vs
	}
	return vs
}

// totalRequests returns the sum of RequestsTotal across all vendors.
func (s *Snapshot) totalRequests() float64 {
	var total float64
	for _, vs := range s.Vendors {
		total += vs.RequestsTotal
	}
	return total
}

// totalErrors returns the sum of RequestsErrors across all vendors.
func (s *Snapshot) totalErrors() float64 {
	var total float64
	for _, vs := range s.Vendors {
		total += vs.RequestsErrors
	}
	return total
}

// --- API response types ---

// InstanceMetrics is returned by GET /api/metrics/{id}.
type InstanceMetrics struct {
	InstanceID        int64                          `json:"instance_id"`
	CollectedAt       time.Time                      `json:"collected_at"`
	DataPoints        int                            `json:"data_points"`
	RPS               float64                        `json:"rps"`
	ErrorRate         float64                        `json:"error_rate"`
	ActiveConnections float64                        `json:"active_connections"`
	PanicsTotal       float64                        `json:"panics_total"`
	P50               float64                        `json:"p50_ms"`
	P95               float64                        `json:"p95_ms"`
	P99               float64                        `json:"p99_ms"`
	RPSTrend          *float64                       `json:"rps_trend"`
	ErrorRateTrend    *float64                       `json:"error_rate_trend"`
	Vendors           []VendorMetrics                `json:"vendors"`
	Series            []SeriesPoint                  `json:"series"`
	VendorSeries      map[string][]VendorSeriesPoint `json:"vendor_series"`
}

// FleetMetrics is returned by GET /api/metrics/fleet.
type FleetMetrics struct {
	CollectedAt            time.Time         `json:"collected_at"`
	TotalRPS               float64           `json:"total_rps"`
	FleetErrorRate         float64           `json:"fleet_error_rate"`
	TotalActiveConnections float64           `json:"total_active_connections"`
	TotalPanics            float64           `json:"total_panics"`
	RPSTrend               *float64          `json:"rps_trend"`
	ErrorRateTrend         *float64          `json:"error_rate_trend"`
	Instances              []InstanceSummary `json:"instances"`
}

// InstanceSummary is a compact per-instance overview for the fleet endpoint.
type InstanceSummary struct {
	InstanceID        int64   `json:"instance_id"`
	RPS               float64 `json:"rps"`
	ErrorRate         float64 `json:"error_rate"`
	ActiveConnections float64 `json:"active_connections"`
	PanicsTotal       float64 `json:"panics_total"`
	P99               float64 `json:"p99_ms"`
}

// VendorMetrics holds current per-vendor KPIs.
type VendorMetrics struct {
	VendorID  string  `json:"vendor_id"`
	RPS       float64 `json:"rps"`
	ErrorRate float64 `json:"error_rate"`
	P50       float64 `json:"p50_ms"`
	P95       float64 `json:"p95_ms"`
	P99       float64 `json:"p99_ms"`
}

// SeriesPoint is one data point in a global time series.
type SeriesPoint struct {
	Time              int64   `json:"t"`
	RPS               float64 `json:"rps"`
	ErrorRate         float64 `json:"err"`
	P99               float64 `json:"p99"`
	ActiveConnections float64 `json:"conn"`
}

// VendorSeriesPoint is one data point in a per-vendor time series.
type VendorSeriesPoint struct {
	Time      int64   `json:"t"`
	RPS       float64 `json:"rps"`
	ErrorRate float64 `json:"err"`
	P50       float64 `json:"p50"`
	P95       float64 `json:"p95"`
	P99       float64 `json:"p99"`
}
