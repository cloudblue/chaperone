# Task: Context Hashing

**Status:** [ ] Not Started  
**Priority:** P1  
**Estimated Effort:** M (Medium)

## Objective

Implement deterministic hashing of `TransactionContext` to generate cache keys for the Fast Path caching strategy.

## Design Spec Reference

- **Primary:** Section 5.2.B - Routing & Injection Logic (Step 2: Cache Lookup)
- **Primary:** ADR-003 - Hybrid Caching Strategy
- **Related:** Section 7 - TransactionContext struct

## Dependencies

- [x] `02-transaction-context.task.md` - TransactionContext struct exists

## Acceptance Criteria

- [ ] Function `HashContext(ctx *sdk.TransactionContext) string` exists
- [ ] Hash is deterministic: same input → same output across restarts
- [ ] Hash is unique: different inputs → different outputs (collision-resistant)
- [ ] Field order doesn't affect hash (canonicalization)
- [ ] Handles nil/empty fields gracefully
- [ ] Tests pass: `go test ./internal/cache/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Create `internal/cache/` package
2. Write tests first (TDD):
   - Same context → same hash
   - Different context → different hash
   - Empty fields handled
   - Field order invariant (if using map in Data)
3. Implement hasher using SHA-256

### Key Code Location

```
internal/
└── cache/
    ├── hash.go       # HashContext function
    └── hash_test.go  # Tests
```

### Canonicalization Strategy

```go
// 1. Create deterministic string representation
// 2. Sort map keys for Data field
// 3. Use consistent format: "field1:value1|field2:value2|..."
// 4. SHA-256 hash the canonical string
// 5. Return hex-encoded hash
```

### Algorithm

```go
func HashContext(ctx *sdk.TransactionContext) string {
    // Handle nil
    if ctx == nil {
        return hashEmpty
    }
    
    // Build canonical representation
    var parts []string
    parts = append(parts, "target:"+ctx.TargetURL)
    parts = append(parts, "marketplace:"+ctx.MarketplaceID)
    parts = append(parts, "vendor:"+ctx.VendorID)
    parts = append(parts, "product:"+ctx.ProductID)
    parts = append(parts, "subscription:"+ctx.SubscriptionID)
    
    // Sort and add Data map
    if ctx.Data != nil {
        // Sort keys, serialize values
    }
    
    canonical := strings.Join(parts, "|")
    hash := sha256.Sum256([]byte(canonical))
    return hex.EncodeToString(hash[:])
}
```

### Gotchas

- `Data` map iteration order is random in Go; must sort keys
- Consider whether `Data` values need deep serialization (nested maps)
- Empty string vs missing field: decide on consistent handling
- Don't use time-based components (breaks determinism)

## Files to Create/Modify

- [ ] `internal/cache/hash.go` - Hashing logic
- [ ] `internal/cache/hash_test.go` - Tests

## Testing Strategy

### Unit Tests (Table-Driven)

```go
tests := []struct {
    name    string
    ctx     *sdk.TransactionContext
    wantLen int // SHA-256 hex = 64 chars
}{
    {"nil context", nil, 64},
    {"empty context", &sdk.TransactionContext{}, 64},
    {"full context", fullContext, 64},
}

// Determinism test
func TestHashContext_Deterministic(t *testing.T) {
    ctx := makeTestContext()
    hash1 := HashContext(ctx)
    hash2 := HashContext(ctx)
    if hash1 != hash2 {
        t.Error("hash not deterministic")
    }
}

// Uniqueness test
func TestHashContext_Unique(t *testing.T) {
    ctx1 := &sdk.TransactionContext{VendorID: "a"}
    ctx2 := &sdk.TransactionContext{VendorID: "b"}
    if HashContext(ctx1) == HashContext(ctx2) {
        t.Error("different contexts produced same hash")
    }
}
```

## Notes

This task validates the caching strategy inputs for Phase 3's memguard integration.
The actual cache storage is deferred to Phase 3.
