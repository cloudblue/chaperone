# Task: Router (Allow-List)

**Status:** [x] Completed
**Priority:** P0
**Estimated Effort:** M

## Objective

Implement URL validation that enforces host and path allow-listing with glob pattern support, following "Default Deny" security posture.

## Design Spec Reference

- **Primary:** Section 5.3 - Security Controls (Traffic Validation)
- **Primary:** Section 5.5.A - Configuration (allow_list structure)
- **Related:** Section 5.2.B - Routing & Injection Logic (Step 1: Validation)

## Dependencies

- [x] `01-configuration.task.md` - AllowList comes from config

## Acceptance Criteria

- [x] Target URL host validated against `allow_list` keys
- [x] Target URL path validated against allowed path patterns
- [x] Glob patterns supported: `*` (single-level) and `**` (recursive)
- [x] Domain globs: `.` is separator (e.g., `*.google.com` matches `api.google.com`)
- [x] Path globs: `/` is separator (e.g., `/v1/**` matches `/v1/customers/123`)
- [x] Unknown hosts return `403 Forbidden`
- [x] Disallowed paths return `403 Forbidden`
- [x] Empty allow_list denies all requests (secure default)
- [x] Clear error messages in logs (without leaking sensitive data)
- [x] Tests pass: `go test ./internal/router/...`
- [x] Lint passes: `make lint`
- [x] Glob patterns validated at config load time (fail-fast)

## Implementation Hints

### Suggested Approach

1. Create `internal/router/allowlist.go` with validation logic
2. Implement glob matching with proper separator handling
3. Create middleware that intercepts requests before forwarding
4. Return structured 403 response on validation failure
5. Log validation failures (host/path attempted, not full URL with query params)

### Glob Pattern Rules (from Design Spec §5.3)

```
Domain patterns (separator: .)
  *.google.com       → matches api.google.com, NOT a.b.google.com
  **.amazonaws.com   → matches a.b.c.amazonaws.com

Path patterns (separator: /)
  /v1/customers/*    → matches /v1/customers/123, NOT /v1/customers/123/profile
  /v1/**             → matches /v1/anything/deeply/nested
```

### Middleware Integration

```go
func AllowListMiddleware(cfg *config.Config, next http.Handler) http.Handler {
    validator := NewAllowListValidator(cfg.Upstream.AllowList)
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        targetURL := r.Header.Get(cfg.Upstream.HeaderPrefix + "-Target-URL")
        if err := validator.Validate(targetURL); err != nil {
            // Return 403 with sanitized error
            respondForbidden(w, err)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### Key Code Locations

- `internal/router/allowlist.go` - Core validation logic
- `internal/router/glob.go` - Glob pattern matching
- `internal/router/middleware.go` - HTTP middleware wrapper
- `internal/proxy/server.go` - Middleware integration

### Gotchas

- URL parsing: Use `net/url` for proper parsing, handle edge cases
- Case sensitivity: Hosts are case-insensitive, paths may be case-sensitive
- Trailing slashes: `/api/` vs `/api` - define consistent behavior
- Query params: Should be ignored during path validation
- Port in host: `api.example.com:443` - strip port for matching

## Files to Create/Modify

- [x] `internal/router/allowlist.go` - Validation logic
- [x] `internal/router/glob.go` - Glob pattern matching
- [x] `internal/router/middleware.go` - HTTP middleware
- [x] `internal/router/allowlist_test.go` - Unit tests
- [x] `internal/router/glob_test.go` - Glob pattern tests
- [x] `internal/proxy/server.go` - Wire up middleware
- [x] `internal/config/validate.go` - Glob pattern validation at load time
- [x] `internal/config/config_test.go` - Tests for glob validation
- [x] `internal/proxy/integration_test.go` - Integration tests for allow list

## Testing Strategy

- **Unit tests:**
  - Valid host/path combinations
  - Invalid host rejection
  - Invalid path rejection
  - Glob pattern matching (extensive)
  - Edge cases (ports, trailing slashes, encoded chars)
- **Table-driven tests:** Pattern matching scenarios from Design Spec
- **Security tests:**
  - Path traversal attempts (`/../`)
  - URL encoding bypass attempts
  - Empty/nil allow_list behavior
