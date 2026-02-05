# Task: Error Normalization

**Status:** [x] Completed
**Priority:** P0
**Estimated Effort:** M

## Objective

Implement middleware to intercept upstream 400/500 errors, replacing stack traces and internal details with sanitized JSON responses.

## Design Spec Reference

- **Primary:** Section 5.3 - Security Controls (Error Masking)
- **Related:** Section 5.2.B - Routing & Injection Logic (Step 5: Response Handling)
- **Related:** Section 7 - ResponseModifier interface

## Dependencies

- [x] `01-configuration.task.md` - Error response format may be configurable

## Acceptance Criteria

- [x] Upstream 4xx errors: Body replaced with generic JSON, original logged locally
- [x] Upstream 5xx errors: Body replaced with generic JSON, original logged locally
- [x] Stack traces never returned to client
- [x] Internal error details never returned to client
- [x] Original status code preserved (or mapped to safe equivalent)
- [x] Original error body logged at DEBUG level for Distributor troubleshooting
- [x] JSON response format: `{"error": "...", "trace_id": "...", "status": N}`
- [x] Plugin's `ModifyResponse` runs BEFORE Core sanitization
- [x] Plugin can opt out via `ResponseAction{SkipErrorNormalization: true}`
- [x] SDK updated: `ModifyResponse` returns `(*ResponseAction, error)`
- [x] Tests pass: `go test ./internal/sanitizer/...`
- [x] Lint passes: `make lint`

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
    "trace_id": "abc-123-trace-id",
    "status": 500
}
```

For 4xx errors:
```json
{
    "error": "Request rejected by upstream service",
    "trace_id": "abc-123-trace-id", 
    "status": 400
}
```

### Response Modification Chain

The `ModifyResponse` function in the proxy executes this chain:

1. **Plugin.ModifyResponse** - Returns `*sdk.ResponseAction` or `nil`
2. **Strip sensitive headers** - Always runs (security)
3. **Core.NormalizeError** - Runs unless `action.SkipErrorNormalization == true`

```go
// Plugin can opt out of error normalization:
func (p *MyPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
    // ISV returns structured validation errors on 400 - pass through to Connect
    if resp.StatusCode == http.StatusBadRequest {
        return &sdk.ResponseAction{SkipErrorNormalization: true}, nil
    }
    // Default: let Core sanitize
    return nil, nil
}
```

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

- [x] `internal/sanitizer/error.go` - Error detection and normalization
- [x] `internal/sanitizer/error_test.go` - Unit tests
- [x] `internal/proxy/server.go` - Wire up in response chain (ModifyResponse)
- [x] `internal/proxy/integration_test.go` - Integration tests
- [x] `sdk/plugin.go` - Add ResponseAction struct, update ResponseModifier interface
- [x] `sdk/compliance/verify.go` - Update tests for new signature
- [x] `plugins/reference/reference.go` - Update ModifyResponse signature

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
