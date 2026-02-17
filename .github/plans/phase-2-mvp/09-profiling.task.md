# Task: Profiling Endpoints

**Status:** [x] Completed
**Priority:** P1
**Estimated Effort:** M

## Objective

Implement the admin server infrastructure and optional pprof endpoints with build-time security controls for performance debugging.

## Design Spec Reference

- **Primary:** Section 5.1.C - Admin Endpoints (`/debug/pprof/`)
- **Primary:** Section 9.3.C - Profiling Integration
- **Related:** Section 8.3 - Observability & Telemetry

## Dependencies

- [x] Phase 1 completed (proxy core working)
- [x] No dependencies on other Phase 2 tasks (Workstream B - independent)

## Acceptance Criteria

- [x] Admin server running on `:9090` (localhost only by default)
- [x] `/_ops/health` endpoint always available on admin port
- [x] pprof endpoints available when `enable_profiling` config set (dev builds only)
- [x] **Build-time disabled** in production builds (`make build`)
- [x] Startup warning emitted when profiling is enabled
- [x] All standard pprof endpoints available (`/debug/pprof/*`)
- [x] Tests pass: `go test ./internal/telemetry/...`
- [x] Lint passes: `make lint`

## Implementation Hints

### Architecture Decision: Shared Admin Port (Option C)

pprof shares the admin port (`:9090`) with health and future metrics endpoints, rather than using a standalone port (`:6060`). This simplifies operations and follows production patterns.

### Security Model

| Layer | Control |
|-------|---------|
| Build-time | `allowProfiling` ldflags (prod=false, dev=true) |
| Runtime | `-enable-profiling` CLI flag (requires dev build) |
| Network | Localhost-only binding (`127.0.0.1:9090`) |

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

1. Create `internal/telemetry/admin.go` - Admin server infrastructure
2. Create `internal/telemetry/pprof.go` - pprof registration with build-time control
3. Follow `allowInsecureTargets` pattern from `internal/proxy/security.go`
4. Update Makefile to set `allowProfiling=true` in `build-dev` target
5. Integrate in `cmd/chaperone/main.go` with `-admin-addr` and `-enable-profiling` flags

### Key Code Locations

- `internal/telemetry/admin.go` - New admin server
- `internal/telemetry/pprof.go` - pprof handlers with build-time control
- `internal/proxy/security.go` - Pattern to follow for ldflags
- `cmd/chaperone/main.go` - CLI flag integration

### Security Considerations

- **NEVER** expose admin port to public internet
- Admin port should be firewalled to internal network only
- Log warning at startup when profiling enabled
- Build-time disable prevents pprof even if flag is set in production

### Gotchas

- `net/http/pprof` registers handlers globally on import - use explicit registration
- Block profiling requires `runtime.SetBlockProfileRate(1)` to collect data
- Mutex profiling requires `runtime.SetMutexProfileFraction(1)` to collect data
- Task 07 (Metrics) will later add `/metrics` to this admin server

## What We're NOT Doing

- **YAML config toggle**: CLI flag `-enable-profiling` only
- **Environment variable**: CLI flag is sufficient
- **Standalone pprof port (`:6060`)**: Using shared admin port (`:9090`)
- **Authentication on pprof**: Localhost binding is sufficient

## Files to Create/Modify

- [x] `internal/telemetry/admin.go` - Admin server infrastructure
- [x] `internal/telemetry/admin_test.go` - Admin server tests
- [x] `internal/telemetry/pprof.go` - pprof registration
- [x] `internal/telemetry/pprof_test.go` - pprof tests
- [x] `internal/telemetry/integration_test.go` - Integration tests
- [x] `cmd/chaperone/main.go` - Add flags and start admin server
- [x] `Makefile` - Add `allowProfiling` to LDFLAGS_DEV

## Testing Strategy

- **Unit tests:**
  - Admin server health endpoint
  - `RegisterPprofHandlers` returns false when build-time disabled
  - `RegisterPprofHandlers` returns false when flag not set
  - All pprof endpoints accessible when enabled
- **Integration tests:**
  - Admin server starts and shuts down gracefully
  - pprof returns 200 when enabled, 404 when disabled
  - Health endpoint works regardless of pprof state
- **Build verification:**
  - `make build` produces binary without pprof capability
  - `make build-dev` produces binary with pprof capability

## Implementation Plan Reference

See `.memory/plans/2026-02-03-profiling-pprof-endpoint.md` for detailed implementation with full code examples.
