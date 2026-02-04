# Task: Profiling Endpoints

**Status:** [~] In Progress
**Priority:** P1
**Estimated Effort:** M

## Objective

Enable runtime profiling endpoints (`/debug/pprof/*`) on the admin port for performance debugging and optimization.

## Design Spec Reference

- **Primary:** Section 5.1.C - Admin Endpoints (`/debug/pprof/`)
- **Primary:** Section 9.3.C - Profiling Integration
- **Related:** Section 8.3 - Observability & Telemetry

## Dependencies

- [ ] Phase 1 completed (proxy core working)
- [ ] No dependencies on other Phase 2 tasks (Workstream B - independent)

## Acceptance Criteria

- [ ] pprof endpoints available on admin port (`:9090`)
- [ ] Endpoints disabled by default (`observability.enable_profiling: false`)
- [ ] Startup warning emitted when profiling is enabled
- [ ] Endpoints NOT exposed on traffic port (security)
- [ ] All standard pprof endpoints available
- [ ] Tests pass: `go test ./...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Required Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/debug/pprof/` | Index page |
| `/debug/pprof/profile` | CPU profile (30s default) |
| `/debug/pprof/heap` | Heap memory profile |
| `/debug/pprof/goroutine` | Goroutine stack traces |
| `/debug/pprof/block` | Blocking profile |
| `/debug/pprof/mutex` | Mutex contention |
| `/debug/pprof/threadcreate` | Thread creation |
| `/debug/pprof/trace` | Execution trace |

### Suggested Approach

1. Add `enable_profiling` config option (default: `false`)
2. Create admin server mux on `:9090`
3. Conditionally register pprof handlers
4. Add startup log warning when enabled
5. Ensure traffic port does NOT include pprof routes

### Key Code Locations

- `internal/config/config.go` - Add `Observability.EnableProfiling` field
- `internal/proxy/server.go` - Admin server setup
- `internal/proxy/admin.go` - New file for admin endpoints

### Implementation Pattern

```go
import "net/http/pprof"

func (s *Server) setupAdminServer() *http.Server {
    mux := http.NewServeMux()
    
    // Always available
    mux.HandleFunc("/metrics", s.metricsHandler)
    
    // Conditionally enabled
    if s.config.Observability.EnableProfiling {
        slog.Warn("profiling endpoints enabled - do not expose to public internet")
        mux.HandleFunc("/debug/pprof/", pprof.Index)
        mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
        mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
        mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
        mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
    }
    
    return &http.Server{
        Addr:    s.config.Server.AdminAddr,
        Handler: mux,
    }
}
```

### Config Addition

```yaml
observability:
  log_level: "info"
  enable_profiling: false  # NEW: Enables /debug/pprof on Admin Port
```

### Security Considerations

- **NEVER** expose pprof on traffic port (443)
- Admin port should be firewalled to internal network only
- Log warning at startup when enabled
- Consider adding basic auth in future (out of scope for MVP)

### Gotchas

- `net/http/pprof` registers handlers globally on import - use explicit registration
- Block profiling requires `runtime.SetBlockProfileRate(1)` to collect data
- Mutex profiling requires `runtime.SetMutexProfileFraction(1)` to collect data

## Files to Create/Modify

- [ ] `internal/config/config.go` - Add `EnableProfiling` field
- [ ] `internal/proxy/admin.go` - Create admin server with pprof handlers
- [ ] `internal/proxy/server.go` - Wire up admin server
- [ ] `configs/chaperone.example.yaml` - Document new config option
- [ ] `internal/proxy/admin_test.go` - Test profiling toggle

## Testing Strategy

- **Unit tests:**
  - Config parsing for `enable_profiling`
  - Admin server creation with/without profiling
- **Integration tests:**
  - Verify pprof endpoints return 200 when enabled
  - Verify pprof endpoints return 404 when disabled
  - Verify traffic port does NOT have pprof routes
