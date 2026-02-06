# Task: Security Layer (Redactor & Reflector)

**Status:** [x] Completed
**Priority:** P0
**Estimated Effort:** M

## Objective

Implement the "Redactor" middleware for log sanitization and "Reflector" protection for stripping auth headers from responses.

## Design Spec Reference

- **Primary:** Section 5.3 - Security Controls (Credential Reflection Protection, Sensitive Data Redaction)
- **Primary:** Section 8.3.5 - Structured Logs (Privacy)
- **Related:** Section 5.5.A - Configuration (sensitive_headers list)

## Dependencies

- [x] `01-configuration.task.md` - Sensitive headers list from config

## Acceptance Criteria

### Redactor (Logs)
- [x] Configured headers replaced with `[REDACTED]` in all log output
- [x] Default redaction list: `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-API-Key`
- [x] Custom headers can be added via `observability.sensitive_headers` config
- [x] Request and response bodies excluded from logs by default
- [x] DEBUG mode body logging requires explicit env var AND emits startup warning
- [x] Redaction applies to all log levels

### Reflector (Response Sanitization)
- [x] `Authorization` header stripped from responses before returning to client
- [x] `Proxy-Authorization` stripped from responses
- [x] `Set-Cookie` stripped from responses (if applicable)
- [x] Other injection headers stripped (configurable list)
- [x] Plugin's `ModifyResponse` runs BEFORE Reflector (safety net design)
- [x] Dynamically injected headers (per-request) stripped from responses via context propagation

### General
- [x] Tests pass: `go test ./internal/security/...`
- [x] Tests pass: `go test ./internal/observability/...`
- [x] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Create `internal/observability/logger.go` — RedactingHandler (slog wrapper, 5-layer defense)
2. Create `internal/security/reflector.go` — Response header stripping (static + dynamic)
3. Integrate RedactingHandler via `observability.NewLogger()` factory in main
4. Integrate Reflector as final step in response chain (after Plugin.ModifyResponse)
5. Wire `WithSecretValue()` and `WithInjectedHeaders()` in credential injection

### RedactingHandler Implementation (5 layers)

```go
// RedactingHandler wraps slog.Handler with 5-layer defense:
//  1. Key-based: sensitive header name as attr key → [REDACTED]
//  2. Type-based: http.Header values → redact sensitive entries
//  3. Value-based: attr contains known secret from context → [REDACTED]
//  4. Message: log message scanned for context secrets → replaced
//  5. Body suppression: body-related keys redacted when body logging disabled
type RedactingHandler struct {
    inner              slog.Handler
    sensitive          map[string]struct{}
    bodyLoggingEnabled bool
}
```

### Reflector Implementation

```go
// Static stripping: configurable list of well-known sensitive headers
type Reflector struct {
    sensitiveHeaders map[string]struct{}
}
func (s *Reflector) StripResponseHeaders(headers http.Header) { ... }

// Dynamic stripping: per-request injected headers via context
func WithInjectedHeaders(ctx context.Context, keys []string) context.Context { ... }
func StripInjectedHeaders(ctx context.Context, headers http.Header) { ... }
```

### Middleware Chain Order

```
Response from Upstream
  → Plugin.ModifyResponse (optional customization)
  → Reflector static stripping (well-known sensitive headers)
  → Reflector dynamic stripping (per-request injected headers via context)
  → ErrorNormalizer (sanitize error bodies, if not opted out)
  → Client
```

### Key Code Locations

- `internal/observability/logger.go` - RedactingHandler (5-layer slog wrapper)
- `internal/security/reflector.go` - Response header stripping (static + dynamic)
- `internal/proxy/server.go` - Wiring: credential injection, Reflector, response chain
- `internal/config/defaults.go` - DefaultSensitiveHeaders()

### Gotchas

- Case sensitivity: HTTP headers are case-insensitive; use `http.CanonicalHeaderKey`
- slog performance: Redaction should not significantly impact logging performance
- Debug mode: Body logging MUST require explicit opt-in AND log a warning at startup
- Completeness: The Reflector is a safety net; it runs even if Plugin claims to handle sanitization

## Files to Create/Modify

- [x] `internal/observability/logger.go` - RedactingHandler (5-layer slog wrapper)
- [x] `internal/observability/logger_test.go` - 30+ unit tests, 2 fuzz targets
- [x] `internal/security/reflector.go` - Response header stripping (static + dynamic injection)
- [x] `internal/security/reflector_test.go` - Static + dynamic stripping tests
- [x] `internal/proxy/server.go` - Wire Reflector, WithSecretValue, WithInjectedHeaders
- [x] `internal/proxy/integration_test.go` - Integration tests for response sanitization
- [x] `internal/config/config.go` - EnableBodyLogging field (yaml:"-")
- [x] `internal/config/loader.go` - CHAPERONE_OBSERVABILITY_ENABLE_BODY_LOGGING env var
- [x] `cmd/chaperone/main.go` - Wire RedactingHandler via NewLogger, startup warning

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
