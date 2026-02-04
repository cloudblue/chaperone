# Task: Performance Attribution (Server-Timing)

**Status:** [ ] Not Started
**Priority:** P1
**Estimated Effort:** M

## Objective

Implement `Server-Timing` header in responses to allow upstream platforms to understand where time was spent (plugin, upstream, proxy overhead).

## Design Spec Reference

- **Primary:** Section 9.3.D - Performance Attribution
- **Primary:** Section 8.3.3 - Performance Attribution (Server-Timing)
- **Related:** Section 5.2.B - Response Handling

## Dependencies

- [ ] Phase 1 completed (proxy core working)
- [ ] No dependencies on other Phase 2 tasks (Workstream B - independent)

## Acceptance Criteria

- [ ] `Server-Timing` header present in all proxy responses
- [ ] Header contains `plugin`, `upstream`, and `overhead` metrics
- [ ] Timing values are in milliseconds with 2 decimal precision
- [ ] Zero overhead when timing is disabled (config option)
- [ ] Timing survives error responses
- [ ] Tests pass: `go test ./...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Header Format (W3C Server-Timing Standard)

```
Server-Timing: plugin;dur=150.25, upstream;dur=320.50, overhead;dur=2.10
```

**Interpretation:**
- `plugin`: Time spent executing Distributor's custom logic (GetCredentials, Vault lookups)
- `upstream`: Time spent waiting for ISV/Vendor to reply
- `overhead`: Time spent in Proxy Core (mTLS, serialization, routing)

### Suggested Approach

1. Create `TimingRecorder` struct to track durations
2. Initialize recorder at request start
3. Record plugin duration around `GetCredentials` call
4. Record upstream duration around HTTP roundtrip
5. Calculate overhead as `total - plugin - upstream`
6. Add header in response modifier chain

### Key Code Locations

- `internal/timing/recorder.go` - New file for timing logic
- `internal/proxy/middleware.go` - Wrap handlers with timing
- `internal/proxy/handler.go` - Record specific phases

### Implementation Pattern

```go
// internal/timing/recorder.go
package timing

import (
    "fmt"
    "time"
)

// Recorder tracks time spent in different phases of request processing.
type Recorder struct {
    start    time.Time
    plugin   time.Duration
    upstream time.Duration
}

// New creates a new timing recorder, starting the clock immediately.
func New() *Recorder {
    return &Recorder{
        start: time.Now(),
    }
}

// RecordPlugin records the duration of plugin execution.
func (r *Recorder) RecordPlugin(d time.Duration) {
    r.plugin = d
}

// RecordUpstream records the duration of upstream request.
func (r *Recorder) RecordUpstream(d time.Duration) {
    r.upstream = d
}

// Header returns the Server-Timing header value.
// Durations are in milliseconds with 2 decimal places.
func (r *Recorder) Header() string {
    total := time.Since(r.start)
    overhead := total - r.plugin - r.upstream
    
    // Ensure overhead is not negative (clock skew protection)
    if overhead < 0 {
        overhead = 0
    }
    
    return fmt.Sprintf(
        "plugin;dur=%.2f, upstream;dur=%.2f, overhead;dur=%.2f",
        float64(r.plugin.Microseconds())/1000.0,
        float64(r.upstream.Microseconds())/1000.0,
        float64(overhead.Microseconds())/1000.0,
    )
}

// PluginDuration returns the recorded plugin duration.
func (r *Recorder) PluginDuration() time.Duration {
    return r.plugin
}

// UpstreamDuration returns the recorded upstream duration.
func (r *Recorder) UpstreamDuration() time.Duration {
    return r.upstream
}

// TotalDuration returns the total elapsed time since recorder creation.
func (r *Recorder) TotalDuration() time.Duration {
    return time.Since(r.start)
}
```

### Usage in Request Handler

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Start timing
    timer := timing.New()
    
    // ... parse context, validate allow-list ...
    
    // Time plugin execution
    pluginStart := time.Now()
    cred, err := h.plugin.GetCredentials(ctx, txCtx, r)
    timer.RecordPlugin(time.Since(pluginStart))
    
    // ... inject credentials ...
    
    // Time upstream request
    upstreamStart := time.Now()
    resp, err := h.client.Do(proxyReq)
    timer.RecordUpstream(time.Since(upstreamStart))
    
    // ... process response ...
    
    // Add Server-Timing header before writing response
    w.Header().Set("Server-Timing", timer.Header())
    
    // ... write response ...
}
```

### Config Option (Optional Enhancement)

```yaml
observability:
  enable_server_timing: true  # Default: true
```

### Gotchas

- Time should be recorded even on error paths
- Use monotonic clock (`time.Now()` in Go provides this)
- Protect against negative overhead (clock skew)
- Header must be set BEFORE writing response body

## Files to Create/Modify

- [ ] `internal/timing/recorder.go` - Timing recorder implementation
- [ ] `internal/timing/recorder_test.go` - Unit tests
- [ ] `internal/proxy/handler.go` - Integrate timing into request flow
- [ ] `internal/proxy/middleware.go` - Ensure header is added

## Testing Strategy

- **Unit tests:**
  - `Recorder.Header()` format validation
  - Zero duration handling
  - Negative overhead protection
  - Duration accessors
- **Integration tests:**
  - Response contains `Server-Timing` header
  - Values are reasonable (plugin < total, etc.)
  - Header present on error responses (4xx, 5xx)

## Metrics Integration

The timing values should also feed into Prometheus metrics (Task 07):

```go
// internal/telemetry/metrics.go
var (
    pluginDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "chaperone_plugin_duration_seconds",
            Help:    "Time spent in plugin execution",
            Buckets: prometheus.DefBuckets,
        },
        []string{"vendor_id"},
    )
    
    upstreamDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "chaperone_upstream_duration_seconds",
            Help:    "Time spent waiting for upstream response",
            Buckets: prometheus.DefBuckets,
        },
        []string{"vendor_id", "status"},
    )
)
```

This enables Prometheus queries like:
- `histogram_quantile(0.99, rate(chaperone_plugin_duration_seconds_bucket[5m]))`
- Identify slow plugins vs slow upstreams
