# Task: Resilience

**Status:** [ ] Not Started
**Priority:** P0
**Estimated Effort:** L

## Objective

Implement configurable timeouts, graceful shutdown, and panic recovery middleware for production reliability.

## Design Spec Reference

- **Primary:** Section 8.1 - Resilience & Reliability
- **Related:** Section 5.5.A - Configuration (timeouts)
- **Related:** Section 5.2.B - Routing (context timeout propagation)

## Dependencies

- [ ] `01-configuration.task.md` - Timeout values from config

## Acceptance Criteria

### Timeouts
- [ ] Configurable `connect` timeout (default: 5s) - upstream connection
- [ ] Configurable `read` timeout (default: 30s) - response header wait
- [ ] Configurable `write` timeout (default: 30s) - response write
- [ ] Configurable `idle` timeout (default: 120s) - keep-alive connections
- [ ] Context cancellation propagated to plugins (Design Spec §5.2.B)
- [ ] Timeouts apply to both upstream and downstream connections

### Graceful Shutdown
- [ ] Handle `SIGTERM` and `SIGINT` signals
- [ ] Stop accepting new connections on shutdown signal
- [ ] Allow in-flight requests to complete (configurable timeout, default: 30s)
- [ ] Log shutdown initiation and completion
- [ ] Exit cleanly after drain completes

### Panic Recovery
- [ ] Top-level recovery middleware catches panics in handlers AND plugins
- [ ] Panic logged with stack trace (locally, for Distributor)
- [ ] Return `500 Internal Server Error` to client (sanitized)
- [ ] Server continues running after panic recovery
- [ ] Panic count tracked (for future metrics integration)

### General
- [ ] Tests pass: `go test ./internal/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Update `internal/proxy/server.go` with timeout configuration
2. Create `internal/proxy/shutdown.go` for graceful shutdown logic
3. Create `internal/proxy/recovery.go` for panic recovery middleware
4. Wire signal handling in `cmd/chaperone/main.go`
5. Ensure context with timeout passed to plugin calls

### Server Timeout Configuration

```go
func NewServer(cfg *config.Config) *http.Server {
    return &http.Server{
        Addr:         cfg.Server.Addr,
        Handler:      handler,
        ReadTimeout:  cfg.Upstream.Timeouts.Read,
        WriteTimeout: cfg.Upstream.Timeouts.Write,
        IdleTimeout:  cfg.Upstream.Timeouts.Idle,
    }
}
```

### Upstream Client Timeouts

```go
func NewUpstreamClient(cfg *config.Config) *http.Client {
    return &http.Client{
        Timeout: cfg.Upstream.Timeouts.Read + cfg.Upstream.Timeouts.Write,
        Transport: &http.Transport{
            DialContext: (&net.Dialer{
                Timeout: cfg.Upstream.Timeouts.Connect,
            }).DialContext,
            ResponseHeaderTimeout: cfg.Upstream.Timeouts.Read,
        },
    }
}
```

### Graceful Shutdown

```go
func GracefulShutdown(srv *http.Server, timeout time.Duration) {
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit
    
    slog.Info("shutdown initiated, draining connections...")
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        slog.Error("shutdown error", "error", err)
    }
    slog.Info("shutdown complete")
}
```

### Panic Recovery Middleware

```go
func RecoveryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                stack := debug.Stack()
                slog.Error("panic recovered",
                    "error", err,
                    "stack", string(stack),
                    "trace_id", TraceIDFromContext(r.Context()),
                )
                
                // Return sanitized 500 to client
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusInternalServerError)
                json.NewEncoder(w).Encode(map[string]interface{}{
                    "error":    "Internal server error",
                    "error_id": TraceIDFromContext(r.Context()),
                    "status":   500,
                })
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

### Key Code Locations

- `internal/proxy/server.go` - Server timeout configuration
- `internal/proxy/shutdown.go` - Graceful shutdown logic
- `internal/proxy/recovery.go` - Panic recovery middleware
- `internal/proxy/client.go` - Upstream HTTP client with timeouts
- `cmd/chaperone/main.go` - Signal handling and shutdown orchestration

### Gotchas

- Write timeout: Includes time to write response; be careful with large responses
- Panic in goroutine: Only catches panics in the handler chain, not spawned goroutines
- Context propagation: Plugin context must inherit request timeout
- Shutdown order: Close listeners first, then drain, then exit
- HTTP/2: Some timeout behaviors differ with HTTP/2

## Files to Create/Modify

- [ ] `internal/proxy/server.go` - Add timeout configuration
- [ ] `internal/proxy/shutdown.go` - Graceful shutdown
- [ ] `internal/proxy/recovery.go` - Panic recovery middleware
- [ ] `internal/proxy/client.go` - Upstream client with timeouts
- [ ] `internal/proxy/recovery_test.go` - Recovery tests
- [ ] `internal/proxy/shutdown_test.go` - Shutdown tests
- [ ] `cmd/chaperone/main.go` - Signal handling

## Testing Strategy

- **Unit tests:**
  - Panic recovery catches panic and returns 500
  - Panic recovery logs stack trace
  - Server continues after panic
- **Integration tests:**
  - Timeout triggers on slow upstream (mock)
  - Graceful shutdown drains requests
  - Context cancellation reaches plugin
- **Stress tests:**
  - Multiple concurrent panics
  - Shutdown under load
