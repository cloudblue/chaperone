# Task: Plugin Mechanism Verification

**Status:** [ ] Not Started  
**Priority:** P0  
**Estimated Effort:** S (Small)

## Objective

Verify that the static recompilation architecture works: a single binary can be built that includes both the proxy core and a plugin.

## Design Spec Reference

- **Primary:** ADR-001 - Plugin Architecture via Static Recompilation
- **Primary:** ADR-004 - Split Module Versioning
- **Related:** Section 7 - Implementation Guide (Builder Pattern)

## Dependencies

- [x] `07-reference-plugin.task.md` - Reference plugin implemented
- [x] `08-core-skeleton.task.md` - Core skeleton implemented

## Acceptance Criteria

- [ ] `go build ./cmd/chaperone/...` produces a single binary
- [ ] Binary includes plugin logic (no external .so files)
- [ ] Binary can be executed and responds to health check
- [ ] `go build` works from fresh clone (no local replace directives in final)
- [ ] Tests verify plugin methods are callable
- [ ] `cmd/chaperone/main_test.go` exists with version flag test
- [ ] SDK struct methods (`Credential.IsExpired()`, `Credential.TTL()`) have unit tests

## Implementation Hints

### Current Structure

```
cmd/chaperone/main.go imports:
  - github.com/cloudblue/chaperone/internal/proxy
  - github.com/cloudblue/chaperone/plugins/reference
  - github.com/cloudblue/chaperone/sdk (transitively)
```

### main.go Pattern

```go
package main

import (
    "github.com/cloudblue/chaperone/internal/proxy"
    "github.com/cloudblue/chaperone/plugins/reference"
)

func main() {
    // Initialize plugin
    plugin := reference.NewReferencePlugin("credentials.json")
    
    // Initialize server with plugin
    server := proxy.NewServer(":8080", plugin)
    
    // Start server
    if err := server.Start(); err != nil {
        log.Fatal(err)
    }
}
```

### Verification Steps

1. **Build Test:**
   ```bash
   go build -o bin/chaperone ./cmd/chaperone
   ls -la bin/chaperone  # Single binary exists
   file bin/chaperone    # Executable, not shared library
   ```

2. **Run Test:**
   ```bash
   ./bin/chaperone &
   curl http://localhost:8080/_ops/health
   # Should return {"status": "alive"}
   kill %1
   ```

3. **Plugin Callable Test:**
   Write a test that verifies plugin methods execute within the binary.

### Gotchas

- `go.mod` has `replace` directive for local development; ensure documented
- If using `go:embed` for default config, verify it's included
- CGO disabled by default in Go; verify no CGO dependencies snuck in

## Files to Create/Modify

- [ ] `cmd/chaperone/main.go` - Ensure wiring is complete
- [ ] `cmd/chaperone/main_test.go` - Build verification test
- [ ] `Makefile` - Add `build` target if not present

## Testing Strategy

### Build Test

```go
func TestBuild(t *testing.T) {
    // This test verifies the build works
    // Run: go build -o /tmp/chaperone ./cmd/chaperone
    cmd := exec.Command("go", "build", "-o", "/tmp/chaperone-test", "./cmd/chaperone")
    if err := cmd.Run(); err != nil {
        t.Fatalf("build failed: %v", err)
    }
    
    // Verify binary exists
    if _, err := os.Stat("/tmp/chaperone-test"); os.IsNotExist(err) {
        t.Fatal("binary not created")
    }
    
    // Cleanup
    os.Remove("/tmp/chaperone-test")
}
```

### Plugin Integration Test

```go
func TestPluginIntegration(t *testing.T) {
    plugin := reference.NewReferencePlugin("testdata/credentials.json")
    
    // Verify plugin methods are callable (not nil interface)
    ctx := context.Background()
    tx := sdk.TransactionContext{VendorID: "test"}
    req := httptest.NewRequest("GET", "/", nil)
    
    _, err := plugin.GetCredentials(ctx, tx, req)
    // Error is OK (unknown vendor), but method should be callable
    if err == nil {
        // Or it succeeded with test data
    }
}
```

## Notes

This task validates ADR-001 ("Caddy Model"):
- No external processes
- No shared libraries
- No runtime plugin loading
- Single deployment artifact

Success here proves the architecture is sound.
