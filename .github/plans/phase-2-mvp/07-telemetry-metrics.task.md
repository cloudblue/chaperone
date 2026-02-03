# Task: Telemetry (Prometheus Metrics)

**Status:** [ ] Not Started
**Priority:** P1
**Estimated Effort:** M

## Objective

Expose `/metrics` endpoint with Prometheus counters and histograms for operational monitoring.

## Design Spec Reference

- **Primary:** Section 8.3.2 - Metrics (Performance Telemetry)
- **Primary:** Section 5.1.C - Internal Admin Endpoints (`/metrics`)
- **Related:** Section 3.1 - Component Diagram (Metrics to Prometheus/Datadog)

## Dependencies

- [x] Phase 1 complete - Core proxy skeleton exists
- [ ] **NO dependencies on Tasks 01-06** (enables parallel development)

## Acceptance Criteria

- [ ] `/metrics` endpoint exposed on admin port (default: `:9090`)
- [ ] Prometheus text format output
- [ ] Counter: `chaperone_requests_total{vendor_id, status_code, method}`
- [ ] Histogram: `chaperone_request_duration_seconds{vendor_id}`
- [ ] Histogram: `chaperone_upstream_duration_seconds{vendor_id}`
- [ ] Gauge: `chaperone_active_connections`
- [ ] Standard Go runtime metrics included (`go_*`)
- [ ] Admin server runs independently (separate from main traffic port)
- [ ] Tests pass: `go test ./internal/telemetry/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Add `github.com/prometheus/client_golang` dependency (approved per Design Spec)
2. Create `internal/telemetry/metrics.go` with metric definitions
3. Create metrics middleware that wraps request handlers
4. Create admin server that serves `/metrics`
5. Start admin server alongside main server in `cmd/chaperone/main.go`

### Metric Definitions

```go
package telemetry

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    RequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "chaperone_requests_total",
            Help: "Total number of requests processed",
        },
        []string{"vendor_id", "status_code", "method"},
    )

    RequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "chaperone_request_duration_seconds",
            Help:    "Total request duration including plugin and upstream",
            Buckets: prometheus.DefBuckets,
        },
        []string{"vendor_id"},
    )

    UpstreamDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "chaperone_upstream_duration_seconds",
            Help:    "Time spent waiting for upstream response",
            Buckets: prometheus.DefBuckets,
        },
        []string{"vendor_id"},
    )

    ActiveConnections = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "chaperone_active_connections",
            Help: "Number of active connections",
        },
    )
)
```

### Metrics Middleware

```go
func MetricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ActiveConnections.Inc()
        defer ActiveConnections.Dec()
        
        start := time.Now()
        vendorID := r.Header.Get("X-Connect-Vendor-ID")
        
        wrapped := &responseWrapper{ResponseWriter: w, status: 200}
        next.ServeHTTP(wrapped, r)
        
        duration := time.Since(start).Seconds()
        
        RequestsTotal.WithLabelValues(
            vendorID,
            strconv.Itoa(wrapped.status),
            r.Method,
        ).Inc()
        
        RequestDuration.WithLabelValues(vendorID).Observe(duration)
    })
}
```

### Admin Server

```go
func StartAdminServer(addr string) *http.Server {
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())
    mux.HandleFunc("/_ops/health", healthHandler)
    
    srv := &http.Server{Addr: addr, Handler: mux}
    go func() {
        slog.Info("admin server starting", "addr", addr)
        if err := srv.ListenAndServe(); err != http.ErrServerClosed {
            slog.Error("admin server error", "error", err)
        }
    }()
    return srv
}
```

### Key Code Locations

- `internal/telemetry/metrics.go` - Metric definitions
- `internal/telemetry/middleware.go` - Metrics collection middleware
- `internal/telemetry/admin.go` - Admin server with `/metrics`
- `cmd/chaperone/main.go` - Start admin server

### Gotchas

- Label cardinality: Avoid high-cardinality labels (like subscription_id)
- Vendor ID default: Use "unknown" if header missing
- Histogram buckets: Default buckets may need tuning for your latency profile
- Admin port security: Should not be exposed to public internet (per Design Spec §5.1.C)
- No config dependency: Use hardcoded `:9090` initially; config integration can come later

## Files to Create/Modify

- [ ] `internal/telemetry/metrics.go` - Metric definitions
- [ ] `internal/telemetry/middleware.go` - Collection middleware
- [ ] `internal/telemetry/admin.go` - Admin server
- [ ] `internal/telemetry/metrics_test.go` - Unit tests
- [ ] `cmd/chaperone/main.go` - Start admin server

## Testing Strategy

- **Unit tests:**
  - Metric increments correctly
  - Labels populated correctly
  - Duration recorded accurately
- **Integration tests:**
  - `/metrics` endpoint returns Prometheus format
  - Metrics update after requests
  - Admin server runs independently
- **Manual verification:**
  - Prometheus can scrape endpoint
  - Grafana can visualize metrics
