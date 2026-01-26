---
applyTo: "**/*.go"
---

# Go Error Handling Conventions

These conventions apply to all Go code in the Chaperone project.

## Core Principles

1. **Never panic for recoverable errors** - Only panic for programmer errors (unreachable code)
2. **Always wrap errors with context** - Use `fmt.Errorf` with `%w` verb
3. **Define sentinel errors for expected conditions** - Allows callers to check with `errors.Is`

## Patterns

### Error Wrapping

```go
// ✅ Correct: Wrap with context using %w
if err != nil {
    return fmt.Errorf("fetching credentials for vendor %s: %w", vendorID, err)
}

// ❌ Avoid: Losing the original error
if err != nil {
    return fmt.Errorf("fetching credentials failed")
}

// ❌ Avoid: Just returning without context
if err != nil {
    return err
}
```

### Sentinel Errors

```go
// ✅ Correct: Package-level sentinel errors
var (
    ErrCredentialExpired = errors.New("credential expired")
    ErrVendorNotFound    = errors.New("vendor not found")
)

// Usage in caller:
if errors.Is(err, ErrCredentialExpired) {
    // handle specifically
}
```

### Error Checking

```go
// ✅ Correct: Check errors immediately
result, err := someFunction()
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
// use result

// ❌ Never: Ignore errors
result, _ := someFunction()
```

### Multiple Error Aggregation

```go
// ✅ Correct: Use errors.Join for multiple errors (Go 1.20+)
var errs []error
if err := validate1(); err != nil {
    errs = append(errs, err)
}
if err := validate2(); err != nil {
    errs = append(errs, err)
}
if len(errs) > 0 {
    return errors.Join(errs...)
}
```

## Security Considerations

- **Never include credentials in error messages**
- **Sanitize user input before including in errors**
- **Log detailed errors internally, return generic messages externally**

```go
// ✅ Correct: Generic external, detailed internal
slog.Error("authentication failed", "vendor", vendorID, "error", err)
return ErrAuthenticationFailed // Generic to caller
```
