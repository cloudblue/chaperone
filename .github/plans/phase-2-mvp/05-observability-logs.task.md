# Task: Observability (Structured Logs)

**Status:** [ ] Not Started
**Priority:** P0
**Estimated Effort:** M

## Objective

Implement structured JSON logging to STDOUT with trace ID correlation, latency tracking, and header redaction.

## Design Spec Reference

- **Primary:** Section 8.3.5 - Structured Logs (Audit & Debug)
- **Primary:** Section 8.3.1 - Distributed Tracing (Correlation)
- **Related:** Section 5.5.A - Configuration (log_level)

## Dependencies

- [ ] `01-configuration.task.md` - Log level from config
- [ ] `04-security-layer.task.md` - Redactor integration

## Acceptance Criteria

- [ ] JSON format output to STDOUT (one JSON object per line)
- [ ] Configurable log levels: `debug`, `info`, `warn`, `error`
- [ ] Every request log includes: `trace_id`, `latency_ms`, `status`, `method`, `path`
- [ ] Additional fields: `vendor_id`, `service_id`, `client_ip`
- [ ] Trace ID extracted from configurable header (default: `Connect-Request-ID`)
- [ ] If no trace ID header present, generate UUIDv4
- [ ] Generated trace IDs propagated to downstream requests
- [ ] Sensitive headers redacted (integration with Redactor)
- [ ] Startup logs include version, config path, listening addresses
- [ ] Tests pass: `go test ./internal/observability/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Create `internal/observability/logger.go` with slog JSON handler setup
2. Create request logging middleware that captures timing
3. Extract/generate trace ID early in request lifecycle
4. Store trace ID in request context for propagation
5. Wrap slog handler with Redactor from task 04
6. Configure log level from config

### Log Format (from Design Spec §8.3.5)

```json
{
    "time": "2026-02-02T14:30:00Z",
    "level": "INFO",
    "msg": "request completed",
    "trace_id": "abc-123-def-456",
    "method": "POST",
    "path": "/proxy",
    "status": 200,
    "latency_ms": 145,
    "vendor_id": "microsoft",
    "client_ip": "10.0.0.1"
}
```

### Trace ID Middleware

```go
func TraceIDMiddleware(cfg *config.Config, next http.Handler) http.Handler {
    headerName := cfg.Upstream.TraceHeader // Default: "Connect-Request-ID"
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        traceID := r.Header.Get(headerName)
        if traceID == "" {
            traceID = uuid.NewString()
        }
        ctx := context.WithValue(r.Context(), TraceIDKey, traceID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Request Logger Middleware

```go
func RequestLoggerMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        wrapped := &responseWrapper{ResponseWriter: w}
        
        next.ServeHTTP(wrapped, r)
        
        logger.Info("request completed",
            "trace_id", TraceIDFromContext(r.Context()),
            "method", r.Method,
            "path", r.URL.Path,
            "status", wrapped.status,
            "latency_ms", time.Since(start).Milliseconds(),
            "vendor_id", r.Header.Get("X-Connect-Vendor-ID"),
            "client_ip", clientIP(r),
        )
    })
}
```

### Key Code Locations

- `internal/observability/logger.go` - Logger setup and configuration
- `internal/observability/trace.go` - Trace ID extraction and propagation
- `internal/observability/middleware.go` - Request logging middleware
- `cmd/chaperone/main.go` - Logger initialization

### Gotchas

- Context propagation: Trace ID must flow through entire request lifecycle
- Response capture: Need wrapper to capture status code for logging
- Performance: Logging should be non-blocking (consider async handler for high volume)
- IP extraction: Handle X-Forwarded-For, X-Real-IP for proxied requests
- UUID generation: Use crypto-secure UUIDs, not timestamp-based

## Files to Create/Modify

- [ ] `internal/observability/logger.go` - Logger setup
- [ ] `internal/observability/trace.go` - Trace ID handling
- [ ] `internal/observability/middleware.go` - Request logging
- [ ] `internal/observability/logger_test.go` - Unit tests
- [ ] `internal/observability/middleware_test.go` - Middleware tests
- [ ] `cmd/chaperone/main.go` - Startup logging

## Testing Strategy

- **Unit tests:**
  - JSON format validation
  - Log level filtering
  - Trace ID extraction
  - Trace ID generation when missing
  - Field presence validation
- **Integration tests:**
  - Request flow with trace ID propagation
  - Log output capture and verification
- **Performance tests:**
  - Logging overhead measurement
