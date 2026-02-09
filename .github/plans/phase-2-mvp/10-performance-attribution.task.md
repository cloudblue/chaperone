# Task: Performance Attribution (Server-Timing)

**Status:** [x] Completed
**Priority:** P1
**Estimated Effort:** M

## Objective

Implement `Server-Timing` header in responses to allow upstream platforms to understand where time was spent (plugin, upstream, proxy overhead).

## Design Spec Reference

- **Primary:** Section 9.3.D - Performance Attribution
- **Primary:** Section 8.3.3 - Performance Attribution (Server-Timing)
- **Related:** Section 5.2.B - Response Handling

## Dependencies

- [x] Phase 1 completed (proxy core working)
- [x] No dependencies on other Phase 2 tasks (Workstream B - independent)

## Acceptance Criteria

- [x] `Server-Timing` header present in all proxy responses
- [x] Header contains `plugin`, `upstream`, and `overhead` metrics
- [x] Timing values are in milliseconds with 2 decimal precision
- [x] Always-on (no config toggle — adds negligible overhead: ~4 `time.Now()` calls per request)
- [x] Timing survives error responses (400, 403, 500, 502, 504, panics)
- [x] Tests pass: `go test ./...`
- [x] Lint passes: `make lint`
- [x] Race detector clean: `go test -race`

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

### Config Option (Not Implemented)

Always-on by design. The overhead is negligible (~4 `time.Now()` calls at ~20ns each per request) and the header provides observability value on every response. A config toggle would add complexity for no practical benefit.

### Gotchas

- Time should be recorded even on error paths
- Use monotonic clock (`time.Now()` in Go provides this)
- Protect against negative overhead (clock skew)
- Header must be set BEFORE writing response body
- **Critical:** Upstream duration MUST be recorded inside `ModifyResponse`/`ErrorHandler`, NOT after `proxy.ServeHTTP()` returns — `httputil.ReverseProxy` calls `WriteHeader` internally before returning, so post-ServeHTTP recording would always be zero
- Response writer wrappers need `Unwrap()` for `http.ResponseController` compatibility (Go 1.20+)

## Files to Create/Modify

- [x] `internal/timing/recorder.go` - Timing recorder, context helpers, `Header()`
- [x] `internal/timing/recorder_test.go` - 10 unit tests
- [x] `internal/timing/middleware.go` - `timingResponseWriter`, `WithTiming` middleware
- [x] `internal/timing/middleware_test.go` - 7 unit tests
- [x] `internal/proxy/server.go` - Timing middleware in chain, plugin/upstream instrumentation, extracted `modifyResponse`
- [x] `internal/proxy/middleware.go` - Added `Unwrap()` to existing `responseWriter`
- [x] `internal/proxy/integration_test.go` - 10 new Server-Timing integration tests

## Testing Strategy

- **Unit tests (17 total):**
  - `Recorder.Header()` format validation and decimal precision
  - Zero duration handling
  - Negative overhead protection (clock skew clamping)
  - Duration accessors and `durationToMS` conversion table
  - Context round-trip (`WithRecorder`/`FromContext`)
  - `timingResponseWriter`: header injection, error paths, streaming, implicit `WriteHeader`, `Unwrap()`
- **Integration tests (10 total):**
  - Response contains `Server-Timing` header on success (200)
  - Header present on upstream error (500), bad request (400), AllowList rejection (403), bad gateway (502), plugin timeout (504)
  - Plugin duration reflects actual execution time (50ms sleep, 40-500ms bounds)
  - Upstream duration reflects actual latency (30ms sleep, 20-500ms bounds)
  - No plugin shows `plugin;dur=0.00`
  - Plugin error shows `upstream;dur=0.00` (upstream never reached)

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
