---
applyTo: "**/*_test.go"
---

# Go Testing Conventions

These conventions apply to all test files in the Chaperone project.

## Core Principles

1. **TDD First** - Write failing test before implementation
2. **Table-Driven Tests** - Prefer for multiple scenarios
3. **Descriptive Names** - `TestFunction_Scenario_ExpectedBehavior`
4. **Arrange-Act-Assert** - Clear test structure

## Test Structure

### Naming Convention

```go
// Pattern: TestFunctionName_Scenario_ExpectedBehavior
func TestValidateTargetURL_BlockedHost_ReturnsError(t *testing.T)
func TestGetCredentials_CacheHit_SkipsPlugin(t *testing.T)
func TestNewProxy_ValidConfig_ReturnsProxy(t *testing.T)
```

### Basic Structure (AAA)

```go
func TestSomething(t *testing.T) {
    // Arrange
    input := "test-input"
    expected := "expected-output"
    
    // Act
    result, err := FunctionUnderTest(input)
    
    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != expected {
        t.Errorf("got %q, want %q", result, expected)
    }
}
```

### Table-Driven Tests

```go
func TestValidateTargetURL(t *testing.T) {
    tests := []struct {
        name      string
        url       string
        allowList map[string][]string
        wantErr   bool
        errType   error // Optional: specific error to check
    }{
        {
            name:      "valid exact match",
            url:       "https://api.vendor.com/v1",
            allowList: map[string][]string{"api.vendor.com": {"/v1"}},
            wantErr:   false,
        },
        {
            name:      "blocked host returns error",
            url:       "https://evil.com/data",
            allowList: map[string][]string{"api.vendor.com": {"/**"}},
            wantErr:   true,
            errType:   ErrHostNotAllowed,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateTargetURL(tt.url, tt.allowList)
            
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            
            if tt.errType != nil && !errors.Is(err, tt.errType) {
                t.Errorf("error type = %T, want %T", err, tt.errType)
            }
        })
    }
}
```

## Test Helpers

### Setup/Teardown

```go
func TestWithSetup(t *testing.T) {
    // Setup
    cleanup := setupTestEnvironment(t)
    defer cleanup()
    
    // Test code...
}

func setupTestEnvironment(t *testing.T) func() {
    t.Helper()
    // setup code
    return func() {
        // cleanup code
    }
}
```

### Test Helpers Mark

```go
func assertNoError(t *testing.T, err error) {
    t.Helper() // Marks as helper - errors report caller's line
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

## Plugin Compliance Testing

For SDK plugin implementations, use the compliance suite:

```go
func TestMyPlugin_Compliance(t *testing.T) {
    plugin := &MyPlugin{}
    compliance.VerifyContract(t, plugin)
}
```

## Security Test Patterns

```go
// Test that sensitive data is redacted
func TestLogOutput_SensitiveHeaders_AreRedacted(t *testing.T) {
    // Capture log output
    // Verify no credentials appear
}

// Test allow-list enforcement
func TestProxy_UnauthorizedHost_Returns403(t *testing.T) {
    // Attempt request to non-allowed host
    // Assert 403 response
}
```

## Integration Tests Location

- Unit tests: Same directory as code (`*_test.go`)
- Integration tests: `test/integration/`
- Compliance tests: `sdk/compliance/`
