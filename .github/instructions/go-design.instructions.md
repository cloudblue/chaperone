---
applyTo: "**/*.go"
---

# Go Design Principles

These principles ensure code remains focused, testable, and maintainable.

## SOLID Principles in Go

### Single Responsibility (SRP)

Each function, type, and package should have ONE reason to change.

```go
// ✅ Good: Focused types with single responsibility
type CredentialStore interface {
    Get(ctx context.Context, key string) (*Credential, error)
}

type CredentialCache struct {
    store CredentialStore  // Delegates storage
    ttl   time.Duration
}

// ❌ Avoid: Type doing too many things
type Manager struct {
    // Handles storage, caching, validation, AND notifications
}
```

### Open/Closed Principle (OCP)

Types should be open for extension, closed for modification. Use interfaces.

```go
// ✅ Good: Extend behavior via interface implementations
type Validator interface {
    Validate(data []byte) error
}

func Process(v Validator, data []byte) error {
    if err := v.Validate(data); err != nil {
        return err
    }
    // Process...
}

// New validation rules = new Validator implementation, not code changes
```

### Liskov Substitution (LSP)

Implementations must honor the contract of their interfaces.

```go
// Interface contract: Get returns ErrNotFound if key doesn't exist
type Store interface {
    Get(key string) (string, error)
}

// ✅ All implementations must return ErrNotFound consistently
// ❌ Don't return nil, "" or panic - breaks substitutability
```

### Interface Segregation (ISP)

Prefer small, focused interfaces over large ones.

```go
// ✅ Good: Small, composable interfaces
type Reader interface {
    Read(ctx context.Context, key string) ([]byte, error)
}

type Writer interface {
    Write(ctx context.Context, key string, data []byte) error
}

type ReadWriter interface {
    Reader
    Writer
}

// ❌ Avoid: Large interfaces that force unnecessary implementations
type Store interface {
    Read, Write, Delete, List, Watch, Backup, Restore...
}
```

### Dependency Inversion (DIP)

Depend on abstractions (interfaces), not concrete types.

```go
// ✅ Good: Accept interface, return concrete
func NewService(store Store) *Service {
    return &Service{store: store}
}

// ❌ Avoid: Depending on concrete types
func NewService(store *PostgresStore) *Service { ... }
```

## Function Design

### Size Limits (enforced by linters)

- **Lines**: ≤60 per function
- **Statements**: ≤40 per function  
- **Cognitive Complexity**: ≤15

### Signs a Function Needs Splitting

- Multiple comments explaining "phases" (`// Step 1`, `// Step 2`)
- Nested conditionals (if inside if inside if)
- Multiple error handling blocks with different behaviors
- Both setup AND execution logic in same function
- Hard to name concisely (does too much)

### Extraction Patterns

```go
// ✅ Return early on error
func validate(cfg Config) error {
    if cfg.Timeout <= 0 {
        return errors.New("timeout must be positive")
    }
    if cfg.MaxRetries < 0 {
        return errors.New("max retries cannot be negative")
    }
    return nil
}

// ✅ Pure functions where possible (easier to test)
func formatDuration(d time.Duration) string {
    if d < time.Second {
        return fmt.Sprintf("%dms", d.Milliseconds())
    }
    return fmt.Sprintf("%.1fs", d.Seconds())
}

// ✅ Coordinator delegates to focused helpers
func (s *Service) Process(ctx context.Context, req Request) (*Response, error) {
    if err := s.validate(req); err != nil {
        return nil, fmt.Errorf("validation: %w", err)
    }
    
    data, err := s.fetch(ctx, req.ID)
    if err != nil {
        return nil, fmt.Errorf("fetch: %w", err)
    }
    
    return s.transform(data), nil
}
```

## Package Design

- **One package = one concept** (e.g., `cache`, `config`, `proxy`)
- **Avoid circular dependencies** - if A imports B, B cannot import A
- **Internal packages** for implementation details (`internal/`)
- **Accept interfaces, return structs** at package boundaries

## Linter Enforcement

The following linters help enforce these patterns:

- `funlen` - Flags functions over 60 lines or 40 statements
- `gocognit` - Flags cognitive complexity over 15

If a function triggers these linters, consider:
1. Extracting helper functions
2. Using early returns to reduce nesting
3. Breaking into a "coordinator" + "workers" pattern
