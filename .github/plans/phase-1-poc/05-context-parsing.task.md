# Task: Context Parsing

**Status:** [ ] Not Started  
**Priority:** P0  
**Estimated Effort:** M (Medium)

## Objective

Implement extraction of `X-Connect-*` headers from incoming requests into a `TransactionContext` struct.

## Design Spec Reference

- **Primary:** Section 5.2.A - Inbound Context (From Connect)
- **Related:** Section 5.5.A - Configuration (header_prefix)
- **Related:** Section 7 - TransactionContext struct definition

## Dependencies

- [x] `02-transaction-context.task.md` - TransactionContext struct exists in `sdk/context.go`

## Acceptance Criteria

- [ ] Function `ParseContext(req *http.Request, prefix string) (*sdk.TransactionContext, error)` exists
- [ ] Extracts all standard headers: `Target-URL`, `Marketplace-ID`, `Vendor-ID`, `Product-ID`, `Subscription-ID`
- [ ] Decodes `Context-Data` from Base64 JSON into `Data` map
- [ ] Returns appropriate errors for:
  - Missing required headers (Target-URL is required)
  - Malformed Base64 in Context-Data
  - Invalid JSON in Context-Data
- [ ] Configurable header prefix (default: `X-Connect`)
- [ ] Tests pass: `go test ./internal/context/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Create `internal/context/` package
2. Write tests first (TDD):
   - Valid headers → parsed context
   - Missing Target-URL → error
   - Malformed Base64 → error
   - Invalid JSON → error
   - Empty Context-Data → empty map (not error)
3. Implement parser

### Key Code Location

```
internal/
└── context/
    ├── parser.go       # ParseContext function
    └── parser_test.go  # Table-driven tests
```

### Header Mapping

| Header | TransactionContext Field |
|--------|-------------------------|
| `{prefix}-Target-URL` | `TargetURL` |
| `{prefix}-Marketplace-ID` | `MarketplaceID` |
| `{prefix}-Vendor-ID` | `VendorID` |
| `{prefix}-Product-ID` | `ProductID` |
| `{prefix}-Subscription-ID` | `SubscriptionID` |
| `{prefix}-Context-Data` | `Data` (Base64 JSON) |

### Gotchas

- Header names are case-insensitive in HTTP; use `req.Header.Get()` which handles this
- `Context-Data` is optional; don't error if missing
- `Context-Data` if present but empty string should result in empty `Data` map
- Don't log the decoded `Context-Data` - it may contain sensitive info

## Files to Create/Modify

- [ ] `internal/context/parser.go` - Main parsing logic
- [ ] `internal/context/parser_test.go` - Table-driven tests
- [ ] `internal/context/errors.go` - Sentinel errors (optional, can inline)

## Testing Strategy

### Unit Tests (Table-Driven)

```go
tests := []struct {
    name        string
    headers     map[string]string
    prefix      string
    wantErr     bool
    errContains string
}{
    {"valid all headers", ...},
    {"valid minimal (only Target-URL)", ...},
    {"missing Target-URL", ..., true, "Target-URL required"},
    {"malformed Base64", ..., true, "invalid base64"},
    {"invalid JSON", ..., true, "invalid JSON"},
    {"custom prefix", ...},
}
```

### Security Tests

- Verify no panics on malformed input (fuzz candidate for Phase 3)
- Verify errors don't leak internal details
