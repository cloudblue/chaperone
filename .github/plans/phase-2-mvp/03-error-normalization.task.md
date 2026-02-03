# Task: Error Normalization

**Status:** [ ] Not Started
**Priority:** P0
**Estimated Effort:** M

## Objective

Implement middleware to intercept upstream 400/500 errors, replacing stack traces and internal details with sanitized JSON responses.

## Design Spec Reference

- **Primary:** Section 5.3 - Security Controls (Error Masking)
- **Related:** Section 5.2.B - Routing & Injection Logic (Step 5: Response Handling)
- **Related:** Section 7 - ResponseModifier interface

## Dependencies

- [ ] `01-configuration.task.md` - Error response format may be configurable

## Acceptance Criteria

- [ ] Upstream 4xx errors: Body replaced with generic JSON, original logged locally
- [ ] Upstream 5xx errors: Body replaced with generic JSON, original logged locally
- [ ] Stack traces never returned to client
- [ ] Internal error details never returned to client
- [ ] Original status code preserved (or mapped to safe equivalent)
- [ ] Original error body logged at DEBUG level for Distributor troubleshooting
- [ ] `X-Error-ID` header added for correlation (trace ID)
- [ ] JSON response format: `{"error": "...", "error_id": "...", "status": N}`
- [ ] Plugin's `ModifyResponse` runs BEFORE Core sanitization
- [ ] Tests pass: `go test ./internal/sanitizer/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Create `internal/sanitizer/error.go` with error normalization logic
2. Implement as `httputil.ReverseProxy.ModifyResponse` modifier
3. Check response status code and sanitize body if error
4. Log original body at DEBUG level before replacement
5. Preserve status code but replace body content
6. Ensure Plugin's `ModifyResponse` is called first (they may want to customize)

### Response Format

```json
{
    "error": "Upstream service error",
    "error_id": "abc-123-trace-id",
    "status": 500
}
```

For 4xx errors:
```json
{
    "error": "Request rejected by upstream service",
    "error_id": "abc-123-trace-id", 
    "status": 400
}
```

### Middleware Chain Order

```
Request → Plugin.ModifyResponse → Core.ErrorNormalizer → Client
```

The Plugin can customize error responses, but the Core always runs last as a safety net.

### Key Code Locations

- `internal/sanitizer/error.go` - Error normalization logic
- `internal/sanitizer/response.go` - Response body replacement
- `internal/proxy/middleware.go` - Integration point

### Gotchas

- Body consumption: Reading the body consumes it; must replace with new reader
- Streaming responses: Error detection may need to buffer initial response
- Content-Length: Must update after body replacement
- Content-Type: Must set to `application/json`
- Plugin interaction: Plugin may have already modified the response

## Files to Create/Modify

- [ ] `internal/sanitizer/error.go` - Error detection and normalization
- [ ] `internal/sanitizer/response.go` - Response body manipulation
- [ ] `internal/sanitizer/error_test.go` - Unit tests
- [ ] `internal/proxy/middleware.go` - Wire up in response chain

## Testing Strategy

- **Unit tests:**
  - 4xx error normalization
  - 5xx error normalization
  - Stack trace removal
  - JSON body replacement
  - Original body logging
- **Integration tests:**
  - Mock upstream returning errors
  - Verify client sees sanitized response
  - Verify Distributor logs see original
- **Security tests:**
  - Various error body formats (HTML, plain text, JSON with traces)
  - Ensure no information leakage
