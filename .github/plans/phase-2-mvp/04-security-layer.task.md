# Task: Security Layer (Redactor & Reflector)

**Status:** [ ] Not Started
**Priority:** P0
**Estimated Effort:** M

## Objective

Implement the "Redactor" middleware for log sanitization and "Reflector" protection for stripping auth headers from responses.

## Design Spec Reference

- **Primary:** Section 5.3 - Security Controls (Credential Reflection Protection, Sensitive Data Redaction)
- **Primary:** Section 8.3.5 - Structured Logs (Privacy)
- **Related:** Section 5.5.A - Configuration (sensitive_headers list)

## Dependencies

- [ ] `01-configuration.task.md` - Sensitive headers list from config

## Acceptance Criteria

### Redactor (Logs)
- [ ] Configured headers replaced with `[REDACTED]` in all log output
- [ ] Default redaction list: `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-API-Key`
- [ ] Custom headers can be added via `observability.sensitive_headers` config
- [ ] Request and response bodies excluded from logs by default
- [ ] DEBUG mode body logging requires explicit env var AND emits startup warning
- [ ] Redaction applies to all log levels

### Reflector (Response Sanitization)
- [ ] `Authorization` header stripped from responses before returning to client
- [ ] `Proxy-Authorization` stripped from responses
- [ ] `Set-Cookie` stripped from responses (if applicable)
- [ ] Other injection headers stripped (configurable list)
- [ ] Plugin's `ModifyResponse` runs BEFORE Reflector (safety net design)

### General
- [ ] Tests pass: `go test ./internal/sanitizer/...`
- [ ] Tests pass: `go test ./internal/observability/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Create `internal/sanitizer/redactor.go` for log redaction
2. Create `internal/sanitizer/reflector.go` for response header stripping
3. Integrate Redactor with slog handler (custom handler that redacts)
4. Integrate Reflector as final step in response chain
5. Reflector runs AFTER Plugin.ModifyResponse, AFTER ErrorNormalizer

### Redactor Implementation

```go
// RedactingHandler wraps slog.Handler to redact sensitive headers
type RedactingHandler struct {
    inner     slog.Handler
    sensitive map[string]struct{}
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
    // Clone and redact any header-related attributes
    r.Attrs(func(a slog.Attr) bool {
        if a.Key == "headers" {
            // Redact sensitive values
        }
        return true
    })
    return h.inner.Handle(ctx, r)
}
```

### Reflector Implementation

```go
var dangerousResponseHeaders = []string{
    "Authorization",
    "Proxy-Authorization",
    "Set-Cookie",
    "X-API-Key",
    "WWW-Authenticate", // Could leak auth scheme details
}

func StripSensitiveHeaders(resp *http.Response) {
    for _, h := range dangerousResponseHeaders {
        resp.Header.Del(h)
    }
}
```

### Middleware Chain Order

```
Response from Upstream
  → Plugin.ModifyResponse (optional customization)
  → ErrorNormalizer (sanitize error bodies)  
  → Reflector (strip auth headers) ← ALWAYS LAST
  → Client
```

### Key Code Locations

- `internal/sanitizer/redactor.go` - Log redaction logic
- `internal/sanitizer/reflector.go` - Response header stripping
- `internal/observability/logger.go` - slog integration
- `internal/proxy/middleware.go` - Response chain integration

### Gotchas

- Case sensitivity: HTTP headers are case-insensitive; use `http.CanonicalHeaderKey`
- slog performance: Redaction should not significantly impact logging performance
- Debug mode: Body logging MUST require explicit opt-in AND log a warning at startup
- Completeness: The Reflector is a safety net; it runs even if Plugin claims to handle sanitization

## Files to Create/Modify

- [ ] `internal/sanitizer/redactor.go` - Log redaction
- [ ] `internal/sanitizer/reflector.go` - Response header stripping
- [ ] `internal/sanitizer/redactor_test.go` - Redactor tests
- [ ] `internal/sanitizer/reflector_test.go` - Reflector tests
- [ ] `internal/observability/logger.go` - slog handler with redaction
- [ ] `internal/proxy/middleware.go` - Wire up Reflector

## Testing Strategy

- **Unit tests:**
  - Header redaction (various cases)
  - Case-insensitive matching
  - Custom sensitive headers from config
  - Response header stripping
- **Security tests:**
  - Verify Authorization never appears in logs
  - Verify Authorization never returned in response
  - Debug mode warning emission
- **Integration tests:**
  - End-to-end request with sensitive headers
  - Verify logs are redacted
  - Verify response is stripped
