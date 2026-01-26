# Task: Reference Plugin (Complete Implementation)

**Status:** [~] In Progress  
**Priority:** P0  
**Estimated Effort:** M (Medium)

## Objective

Complete the reference plugin implementation that reads credentials from a JSON file, implementing all SDK interfaces for PoC validation.

## Design Spec Reference

- **Primary:** Section 7 - Implementation Guide (ReferenceProvider)
- **Primary:** ADR-001 - Static Recompilation
- **Related:** Section 5.2.B - Routing & Injection Logic (Fast Path vs Slow Path)

## Dependencies

- [x] `04-plugin-interfaces.task.md` - Plugin interfaces defined in `sdk/plugin.go`

## Current State

A partial implementation exists in `plugins/reference/reference.go`:
- Basic file loading
- `GetCredentials` for bearer, api_key, basic auth types

## Acceptance Criteria

- [ ] Implements `sdk.Plugin` interface completely
- [ ] Implements `sdk.CredentialProvider.GetCredentials` (already partial)
- [ ] Implements `sdk.CertificateSigner.SignCSR` (stub returning error for PoC)
- [ ] Implements `sdk.ResponseModifier.ModifyResponse` (no-op for PoC)
- [ ] Loads credentials from JSON file on initialization
- [ ] Returns `*sdk.Credential` for Fast Path (cacheable)
- [ ] Passes compliance suite: `sdk/compliance/verify.go`
- [ ] Tests pass: `go test ./plugins/reference/...`

## Implementation Hints

### Current Structure

```go
// plugins/reference/reference.go
type ReferencePlugin struct {
    credentials map[string]CredentialConfig
}

// Need to add:
func (p *ReferencePlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error)
func (p *ReferencePlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) error
```

### SignCSR Stub (PoC)

For Phase 1, return an error indicating manual cert rotation:

```go
func (p *ReferencePlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
    return nil, fmt.Errorf("certificate signing not implemented in reference plugin: use manual rotation")
}
```

### ModifyResponse No-Op (PoC)

```go
func (p *ReferencePlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) error {
    // No modifications in reference implementation
    return nil
}
```

### Ensure Plugin Interface Compliance

```go
// Verify at compile time
var _ sdk.Plugin = (*ReferencePlugin)(nil)
```

## Files to Create/Modify

- [ ] `plugins/reference/reference.go` - Add missing interface methods
- [ ] `plugins/reference/reference_test.go` - Update tests for full compliance
- [ ] `plugins/reference/testdata/credentials.json` - Ensure test data covers all types

## Testing Strategy

### Compliance Test

```go
func TestReferencePlugin_Compliance(t *testing.T) {
    plugin := NewReferencePlugin("testdata/credentials.json")
    compliance.VerifyContract(t, plugin)
}
```

### Unit Tests

- `GetCredentials` returns correct headers for each auth type
- `GetCredentials` returns error for unknown vendor
- `SignCSR` returns appropriate error
- `ModifyResponse` returns nil (no-op)

### Test Data

Ensure `testdata/credentials.json` has entries for:
- Bearer token auth
- API key auth  
- Basic auth
- Unknown vendor (for error testing)

## Notes

The reference plugin serves two purposes:
1. Default implementation for simple deployments
2. Example for Distributors building custom plugins

Keep it simple and well-documented.
