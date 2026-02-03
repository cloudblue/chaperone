# Task: Module Preparation

**Status:** [ ] Not Started
**Priority:** P1
**Estimated Effort:** S

## Objective

Remove `replace` directives, verify independent module imports, and prepare for future publication.

## Design Spec Reference

- **Primary:** ADR-004 - Split Module Versioning
- **Primary:** Section 5.4 - Versioning & Backward Compatibility
- **Related:** Section 7 - Implementation Guide (Monorepo structure)

## Dependencies

- [ ] All other Phase 2 tasks complete (this is the final task)

## Acceptance Criteria

- [ ] `go.mod` has no `replace` directives
- [ ] `sdk/go.mod` has no `replace` directives
- [ ] Core module can be imported independently: `go get github.com/cloudblue/chaperone`
- [ ] SDK module can be imported independently: `go get github.com/cloudblue/chaperone/sdk`
- [ ] External project can import SDK and implement Plugin interface
- [ ] Version tags follow convention: `v1.x.x` for core, `sdk/v1.x.x` for SDK
- [ ] Go module checksums valid
- [ ] `go mod tidy` produces no changes
- [ ] `go build ./...` succeeds
- [ ] All tests pass: `make test`

## Implementation Hints

### Suggested Approach

1. Review current `go.mod` files for `replace` directives
2. Remove any development-only `replace` directives
3. Ensure proper version requirements between modules
4. Test imports from a separate directory/module
5. Document versioning and release process

### Module Structure (from Design Spec §7)

```
/chaperone
├── go.mod                  # module github.com/cloudblue/chaperone
│                           # requires github.com/cloudblue/chaperone/sdk v1.x.x
└── sdk/
    └── go.mod              # module github.com/cloudblue/chaperone/sdk
                            # no dependencies on parent
```

### Verification Test

Create a test directory outside the repo:

```bash
mkdir /tmp/test-import
cd /tmp/test-import
go mod init test-import

# Test SDK import
cat > main.go << 'EOF'
package main

import (
    "fmt"
    "github.com/cloudblue/chaperone/sdk"
)

func main() {
    fmt.Printf("SDK imported: %T\n", sdk.TransactionContext{})
}
EOF

go mod tidy
go build .
```

### Tagging Strategy

```bash
# For SDK releases (stable interface)
git tag sdk/v1.0.0
git push origin sdk/v1.0.0

# For Core releases (can change frequently)  
git tag v1.0.0
git push origin v1.0.0
```

### Key Code Locations

- `go.mod` - Core module definition
- `sdk/go.mod` - SDK module definition
- `.github/workflows/` - CI/CD for releases (if applicable)

### Gotchas

- Replace directives: May exist for local development; must be removed for release
- Module cache: `go clean -modcache` if testing stale versions
- Private repo: If still private, import tests need authentication
- Circular dependency: SDK must not import Core (it's the other way around)
- Version alignment: Core depends on SDK; version bump SDK first if interface changes

## Files to Create/Modify

- [ ] `go.mod` - Remove replace directives, verify dependencies
- [ ] `sdk/go.mod` - Remove replace directives
- [ ] `docs/releasing.md` - (Optional) Document release process

## Testing Strategy

- **Verification tests:**
  - Import SDK from external module
  - Import Core from external module
  - Build plugin using only SDK
- **CI verification:**
  - `go mod tidy` produces no diff
  - `go mod verify` passes
  - `go build ./...` succeeds
