# Task 13: Module Preparation

**Status:** [~] Pending Review
**Priority:** P1
**Estimated Effort:** M (revised from S — scope expanded to include public API façade)

## Objective

Remove `replace` directives, create public `chaperone.Run()` API, verify independent module imports, and prepare for future publication.

> **Scope Expansion:** An analysis of the public API gap revealed that the
> "Own Repo" Distributor workflow (Design Spec §7) is currently broken because all
> server lifecycle code lives in `internal/`. This task now includes creating a thin
> public `chaperone.Run()` façade to fix this gap. The full analysis is included
> in the [Public API Analysis](#public-api-analysis) section below.

## Design Spec Reference

- **Primary:** ADR-004 - Split Module Versioning
- **Primary:** Section 5.4 - Versioning & Backward Compatibility
- **Primary:** Section 7 - Implementation Guide (Builder Pattern / "Own Repo" workflow)
- **Related:** [Public API Analysis](#public-api-analysis) section below

## Dependencies

- [x] `01-configuration.task.md` - Config structure finalized
- [x] `05-observability-logs.task.md` - Logger API stabilized
- [x] `06-resilience.task.md` - Internal APIs stabilized (shutdown, timeouts)

## Acceptance Criteria

### Module Hygiene
- [~] `go.mod` has no `replace` directives — *pre-publication `replace` retained with documentation; cannot be removed until modules are tagged and published on a Go proxy*
- [x] `sdk/go.mod` has no `replace` directives
- [x] Go module checksums valid (`go mod verify` passes)
- [x] `go mod tidy` produces no changes (both modules)
- [x] `go build ./...` succeeds
- [x] All tests pass: `make test`

### Public API (from scope expansion)
- [x] `chaperone.go` exists with public `Run(plugin sdk.Plugin, opts ...Option)` function
- [x] `Option` type with `WithConfigPath(string)` and `WithVersion(string)` options
- [x] `Run()` delegates to `internal/` packages — thin façade, no reimplemented logic
- [x] `cmd/chaperone/main.go` uses `chaperone.Run()` for server lifecycle
- [x] `chaperone_test.go` verifies `Run()` starts a working proxy

### Distributor Workflow
- [~] SDK module importable independently: `go get github.com/cloudblue/chaperone/sdk` — *validated locally via `replace`; `go get` requires publication*
- [~] Core module importable independently: `go get github.com/cloudblue/chaperone` — *validated locally via `replace`; `go get` requires publication*
- [x] External project can import SDK and implement `Plugin` interface
- [x] External project can import `chaperone.Run()` and build a working binary
- [x] `scripts/test-distributor-workflow.sh` validates the "Own Repo" workflow
- [x] Version tags follow convention: `v1.x.x` for core, `sdk/v1.x.x` for SDK

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

### Create
- [x] `chaperone.go` - Public `Run()` function + `Option` types (~80-120 lines)
- [x] `chaperone_test.go` - Integration tests for `Run()`
- [x] `scripts/test-distributor-workflow.sh` - Validates "Own Repo" import workflow

### Modify
- [x] `go.mod` - Cleaned replace directive with documentation; verify dependencies
- [x] `cmd/chaperone/main.go` - Refactor to use `chaperone.Run()` for server lifecycle

### Unchanged
- `sdk/go.mod` - Already clean (no `replace` directives, no dependencies on parent)
- `sdk/plugin.go` - SDK interfaces are already public and correct
- All `internal/` packages - Stay internal; the façade wraps them, doesn't move them

## Testing Strategy

- **Verification tests:**
  - Import SDK from external module
  - Import Core from external module
  - Build plugin using only SDK
- **CI verification:**
  - `go mod tidy` produces no diff
  - `go mod verify` passes
  - `go build ./...` succeeds

---

## Public API Analysis

> **Source:** This section captures the full analysis of the public API gap,
> included here as the authoritative reference for the scope expansion.

---

### 1. Problem Statement

The Design Spec (Section 7, "The Builder Pattern") explicitly describes that Distributors
create their own repository, import the Chaperone modules, and build a custom binary:

> "The Distributor does not edit the Proxy source code. They create a custom build
> that imports the Core and their own Plugin."

```text
/my-proxy-build
  ├── go.mod             ← Deps: chaperone v1.x, chaperone/sdk v1.0
  ├── main.go            ← Imports Core + registers custom Plugin
  └── plugins/
      └── my_vault.go    ← Distributor Custom Logic
```

**This workflow is currently broken.** Here's why:

#### The `internal/` Visibility Rule

Go enforces a strict convention: packages under an `internal/` directory can only
be imported by code **within the same module**. The Go compiler rejects any external
import attempt — there is no workaround.

Our `cmd/chaperone/main.go` imports these `internal/` packages to wire the proxy:

```go
import (
    "github.com/cloudblue/chaperone/internal/config"         // ❌ Not importable externally
    "github.com/cloudblue/chaperone/internal/observability"   // ❌ Not importable externally
    "github.com/cloudblue/chaperone/internal/proxy"           // ❌ Not importable externally
    "github.com/cloudblue/chaperone/internal/telemetry"       // ❌ Not importable externally
    "github.com/cloudblue/chaperone/plugins/reference"        // ✅ Importable
    "github.com/cloudblue/chaperone/sdk"                      // ✅ Importable (separate module)
)
```

A Distributor's external project **can** import:
- `github.com/cloudblue/chaperone/sdk` — Plugin interfaces, types ✅
- `github.com/cloudblue/chaperone/plugins/reference` — Reference plugin ✅

But **cannot** import:
- `github.com/cloudblue/chaperone/internal/proxy` — Server, config, middleware ❌
- `github.com/cloudblue/chaperone/internal/config` — YAML loader, validation ❌
- `github.com/cloudblue/chaperone/internal/observability` — Redacting logger ❌
- `github.com/cloudblue/chaperone/internal/telemetry` — Admin server, pprof ❌

**Result:** A Distributor can write a plugin (SDK is public), but cannot build a
working binary because the proxy server, config loader, and all operational
infrastructure are locked inside `internal/`.

#### Impact Assessment

| Workflow | Status | Notes |
|----------|--------|-------|
| **Fork/Extend this repo** | ✅ Works | Clone, add plugin, `make build`. Same module boundary. |
| **Own Repo (production)** | ❌ Broken | Cannot import proxy core — compiler rejects it. |
| **SDK import only** | ✅ Works | Can write and test plugins against the interface. |

The "Own Repo" workflow is the **recommended path for production** and the one documented
in Design Spec Section 7. It provides the cleanest version management (ADR-004)
and avoids merge conflicts when upgrading the core. This must work.

---

### 2. Analysis: What Needs to Be Public

A Distributor building their own binary needs exactly **one thing** from us:
a way to start the proxy with their plugin. They should NOT need to understand
config loading, middleware wiring, TLS setup, admin servers, etc.

Looking at `cmd/chaperone/main.go` (209 lines), it does:

1. Parse CLI flags (`-config`, `-credentials`, `-version`)
2. Load config from YAML + env vars (`config.Load()`)
3. Configure structured logging with redaction (`observability.NewLogger()`)
4. Start admin server with health/pprof (`telemetry.NewAdminServer()`)
5. Initialize plugin
6. Create proxy server with full config wiring (`proxy.NewServer()`)
7. Set up signal handling for graceful shutdown
8. Start the server

Most of this is **boilerplate that every Distributor would copy verbatim**. The only
Distributor-specific part is step 5 (which plugin to use).

#### What Should Stay Internal

Everything in `internal/` is correctly placed. These are implementation details
that Distributors should not depend on:

- `internal/proxy` — Reverse proxy internals, middleware chain, TLS loading
- `internal/config` — YAML parsing, validation, defaults merging
- `internal/observability` — Redacting logger implementation
- `internal/context` — Header parsing into `TransactionContext`
- `internal/router` — Allow-list validation, glob matching
- `internal/security` — Error normalization, reflector, header stripping
- `internal/telemetry` — Admin server, pprof registration
- `internal/cache` — Context hashing
- `internal/cli` — Flag parsing

These should remain `internal/` to preserve our freedom to refactor without
breaking Distributor builds.

---

### 3. Proposed Solution: Public `chaperone.Run()` Entry Point

#### Pattern: Thin Public Façade

Create a single public Go file at the module root (`chaperone.go`) that exposes
a high-level entry point. This file delegates 100% of the work to `internal/`
packages — it is a **façade**, not a reimplementation.

This pattern is used by major Go projects:
- **Caddy** exposes `caddy.Run()` while keeping internals private
- **Hugo** exposes `hugo.New()` as the public entry point
- **Terraform** exposes provider interfaces while keeping core internal

#### API Design

```go
// chaperone.go — Public API for Distributors
package chaperone

import "github.com/cloudblue/chaperone/sdk"

// Run starts the Chaperone proxy with the given plugin.
//
// This is the primary entry point for Distributors building custom binaries.
// It handles configuration loading, logging setup, admin server, TLS,
// graceful shutdown, and all operational concerns.
//
// The plugin parameter implements the sdk.Plugin interface to provide
// credential injection logic. Pass nil to run without credential injection.
//
// Run blocks until the server is shut down (via SIGTERM/SIGINT).
// It calls os.Exit(1) on fatal errors.
//
// Example (Distributor's main.go):
//
//   func main() {
//       plugin := &myPlugin{}
//       chaperone.Run(plugin)
//   }
func Run(plugin sdk.Plugin, opts ...Option) {
    // Delegates to internal/ packages — same logic as cmd/chaperone/main.go
}

// Option configures optional behavior for Run.
type Option func(*runConfig)

// WithConfigPath sets the path to the YAML config file.
// Default resolution: -config flag → CHAPERONE_CONFIG env → ./config.yaml
func WithConfigPath(path string) Option { ... }

// WithVersion sets the version string reported by /_ops/version.
func WithVersion(version string) Option { ... }
```

#### What This Enables

A Distributor's `main.go` becomes trivially simple:

```go
package main

import (
    "github.com/cloudblue/chaperone"                     // Public API
    "github.com/acme-corp/acme-proxy/plugins"            // Their plugin
)

func main() {
    plugin := plugins.NewAcmePlugin()
    chaperone.Run(plugin)
}
```

Their `go.mod`:
```go
module github.com/acme-corp/acme-proxy

go 1.25

require (
    github.com/cloudblue/chaperone     v1.5.0   // Core + Run() API
    github.com/cloudblue/chaperone/sdk v1.0.0   // Interfaces (transitive, but pin explicitly)
)
```

Build and deploy:
```bash
go build -o acme-proxy .     # Single binary with everything compiled in
docker build -t acme-proxy . # Using our Dockerfile as template
```

#### What `cmd/chaperone/main.go` Becomes

Our own `cmd/chaperone/main.go` would **use the same public API**:

```go
package main

import (
    "github.com/cloudblue/chaperone"
    "github.com/cloudblue/chaperone/plugins/reference"
)

func main() {
    plugin := reference.New("credentials.json")
    chaperone.Run(plugin)
}
```

This is called **"eating our own dog food"** — our CLI uses the same API
that Distributors use, guaranteeing it works.

> **Note:** `cmd/chaperone/main.go` may keep additional CLI features
> (like the `enroll` subcommand, `--credentials` flag, etc.) that are
> specific to the bundled binary. The core `Run()` function handles the
> common server lifecycle.

---

### 4. Implementation Details

#### 4.1 Files to Create

| File | Purpose |
|------|---------|
| `chaperone.go` | Public `Run()` function + `Option` types |
| `chaperone_test.go` | Tests that `Run()` starts a working proxy |

#### 4.2 Files to Modify

| File | Change |
|------|--------|
| `cmd/chaperone/main.go` | Refactor to use `chaperone.Run()` for the server lifecycle |

#### 4.3 What `chaperone.go` Contains

The implementation is a thin façade (~80-120 lines) that:

1. Parses options
2. Calls `config.Load()` to load YAML + env vars
3. Calls `observability.NewLogger()` to set up structured logging
4. Starts `telemetry.NewAdminServer()` for health/pprof
5. Creates `proxy.NewServer()` with the wired config
6. Sets up signal handling for graceful shutdown
7. Calls `srv.Start()` and blocks

This is essentially **extracting the server lifecycle from `main.go`** into
a public function — no new logic, just exposing existing behavior.

#### 4.4 API Surface Considerations

**What to expose:**
- `Run(plugin, ...Option)` — the primary entry point
- `Option` type + a small set of functional options
- That's it. Keep the public API minimal.

**What NOT to expose:**
- `Config` struct — Distributors configure via YAML/env vars, not Go code
- `Server` type — internal detail
- Middleware — internal detail
- Logger — internal detail

The goal is a **one-function API**. The less we expose, the less we can break.

#### 4.5 Versioning Impact

The public API lives in the **core module** (`github.com/cloudblue/chaperone`),
not the SDK. This means:

- SDK (`sdk/v1.0.0`) — unchanged, stays stable
- Core (`v1.x.x`) — gains the `Run()` function, minor version bump
- No breaking changes to existing code

---

### 5. Relationship to `cmd/chaperone/main.go`

After this change, there are **two entry points**:

| Entry Point | For Whom | What It Does |
|-------------|----------|--------------|
| `chaperone.Run(plugin)` | Distributors (own repo) | Starts proxy with their plugin |
| `cmd/chaperone/main.go` | This repo (fork/extend) | CLI wrapper around `Run()` + extras |

`cmd/chaperone/main.go` continues to provide:
- The `enroll` subcommand for CSR generation
- The `-credentials` flag for the reference plugin
- The `-version` flag
- Custom flag parsing / usage text

But the core server lifecycle (`load config → start servers → graceful shutdown`)
is delegated to `chaperone.Run()`.

---

### 6. Docker & Makefile Support

#### For Fork/Extend Workflow (unchanged)

The existing Dockerfile and Makefile targets work as-is:
```bash
make build          # Builds cmd/chaperone with reference plugin
make docker-build   # Docker image with version injection
make docker-test    # Full validation suite
```

#### For Own Repo Workflow

Distributors can use our Dockerfile as a template. The only change
is the build path:

```dockerfile
# Distributor's Dockerfile (based on ours)
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o proxy .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/proxy /app/proxy
COPY config.yaml /app/config.yaml
USER nonroot:nonroot
EXPOSE 8443 9090
ENTRYPOINT ["/app/proxy"]
CMD ["-config", "/app/config.yaml"]
```

The Makefile targets are specific to this repo's CLI. A Distributor would
have their own build tooling (standard `go build`).

---

### 7. Testing Strategy (Public API)

#### 7.1 Testing Within This Repo

The `chaperone.Run()` function should be tested with integration tests:

```go
// chaperone_test.go
func TestRun_StartsServerAndRespondsToHealth(t *testing.T) {
    // Start Run() in a goroutine with a test config
    // Verify health endpoint responds
    // Send SIGTERM, verify graceful shutdown
}

func TestRun_WithPlugin_InjectsCredentials(t *testing.T) {
    // Start with reference plugin
    // Send proxy request, verify credential injection
}
```

#### 7.2 Testing the "Own Repo" Workflow (E2E)

This is the critical gap: **how do we verify that an external repo can actually
import our modules and build a working binary?**

##### The Problem

Testing this from within the Chaperone repo is inherently difficult:
- Same-module code can always import `internal/` — it won't catch visibility issues
- The `replace` directive in `go.mod` masks import path problems
- We need a **truly external** Go module to validate the Distributor experience

##### Recommended Approach: CI Integration Test

Create a **lightweight CI job** that simulates the Distributor workflow.
This runs as part of our CI pipeline but uses a **separate Go module**:

```yaml
# .github/workflows/distributor-compat.yml
name: Distributor Compatibility

on: [push, pull_request]

jobs:
  test-own-repo:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5

      # Create a temporary external module that imports our public API
      - name: Create test Distributor project
        run: |
          mkdir /tmp/test-distributor
          cd /tmp/test-distributor
          go mod init github.com/test/distributor-proxy

          cat > main.go << 'GOEOF'
          package main

          import (
              "fmt"
              "github.com/cloudblue/chaperone/sdk"
              _ "github.com/cloudblue/chaperone"  // Verify core is importable
          )

          type testPlugin struct{}
          func (p *testPlugin) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
              return nil, nil
          }
          func (p *testPlugin) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
              return nil, nil
          }
          func (p *testPlugin) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
              return nil, nil
          }

          func main() {
              var _ sdk.Plugin = &testPlugin{}
              fmt.Println("Distributor build OK")
          }
          GOEOF

          # Point to local checkout instead of published module
          go mod edit -replace github.com/cloudblue/chaperone=$GITHUB_WORKSPACE
          go mod edit -replace github.com/cloudblue/chaperone/sdk=$GITHUB_WORKSPACE/sdk

          go mod tidy
          go build -o /tmp/test-binary .
          /tmp/test-binary

      - name: Verify binary is functional
        run: |
          echo "External module import: PASS"
```

This approach:
- Uses `go mod edit -replace` to point at the **current checkout** (not a published version)
- Validates the import path is accessible (not blocked by `internal/`)
- Verifies the binary compiles and runs
- Runs on every push/PR — catches regressions immediately

##### Alternative: In-Repo Script

For local development validation (before CI), a script in `scripts/`:

```bash
#!/bin/bash
# scripts/test-distributor-workflow.sh
# Validates that the "Own Repo" workflow works with current code.

set -euo pipefail

TMPDIR=$(mktemp -d)
REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
trap "rm -rf $TMPDIR" EXIT

echo "=== Testing Distributor 'Own Repo' Workflow ==="

cd "$TMPDIR"
go mod init github.com/test/my-proxy

cat > main.go << 'EOF'
package main

import (
    "context"
    "fmt"
    "net/http"
    "github.com/cloudblue/chaperone/sdk"
)

type myPlugin struct{}

func (p *myPlugin) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
    return nil, nil
}
func (p *myPlugin) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
    return nil, nil
}
func (p *myPlugin) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
    return nil, nil
}

func main() {
    var _ sdk.Plugin = &myPlugin{}
    fmt.Println("Build succeeded — Distributor workflow OK")
}
EOF

# Use local checkout
go mod edit -replace "github.com/cloudblue/chaperone=$REPO_ROOT"
go mod edit -replace "github.com/cloudblue/chaperone/sdk=$REPO_ROOT/sdk"
go mod tidy
go build -o "$TMPDIR/test-binary" .
"$TMPDIR/test-binary"

echo "=== PASS ==="
```

> **Note:** This script initially validates SDK-only imports. Once `chaperone.Run()`
> exists, it should be extended to actually start the proxy and hit the health
> endpoint, providing a full end-to-end validation.

##### Why Not a Separate Repository?

A separate `chaperone-template` or `chaperone-integration-test` repo would work
but adds maintenance overhead:
- Must stay in sync with SDK interface changes
- Another repo to manage, tag, and CI
- Easy to forget to update

The CI-based approach keeps the test **co-located** with the code and runs
automatically. If we later decide to publish a template repo for Distributors
(as a convenience), that's additive — but the CI test is the source of truth.

---

### 8. When to Implement

#### Recommendation: Phase 2, Task 13 (Module Preparation)

This fits naturally into **Task 13 - Module Preparation**, which already has these
acceptance criteria:

> - External project can import SDK and implement Plugin interface
> - `go.mod` has no `replace` directives

The `chaperone.Run()` function is the missing piece that makes "external project
can import Core" actually work. Without it, Task 13's acceptance criteria
cannot be fully satisfied.

**Suggested additions to Task 13:**

- [ ] Create `chaperone.go` with public `Run()` function
- [ ] Create `chaperone_test.go` with integration tests
- [ ] Refactor `cmd/chaperone/main.go` to use `Run()` for server lifecycle
- [ ] Add `scripts/test-distributor-workflow.sh` validation script
- [ ] Add CI job for Distributor compatibility testing
- [ ] Verify external project can import `chaperone.Run()` and build a binary

#### Why Not Earlier?

- Tasks 05 (Observability) and 06 (Resilience) are still completing the `internal/`
  packages. The public API should be created after the internal surface stabilizes.
- Task 13 is the first finalization task in Phase 2 — it's the "seal the module" step.
- Creating the public API too early means more churn as internal APIs change.

#### Why Not Later (Phase 3)?

- The "Own Repo" workflow is documented as the **production-recommended path**.
  Shipping Phase 2 (MVP) without it would mean the MVP doesn't support the
  recommended deployment model — a significant gap for early adopter Distributors.
- Task 13 is already scoped for this phase.

---

### 9. Summary

| Question | Answer |
|----------|--------|
| Is the pattern correct? | **Yes.** Design Spec Section 7 describes exactly the right pattern. The gap is purely implementation — no architectural rethink needed. |
| What needs to change? | Create `chaperone.go` with a public `Run(plugin, ...Option)` function. Thin façade over existing `internal/` packages. |
| How much work? | **Small** (~80-120 lines of new code + tests). It's extracting existing `main.go` logic into a public function. |
| When to do it? | **Phase 2, Task 13** (Module Preparation). The internal APIs should stabilize first. |
| How to test it? | CI job that creates a temporary external Go module, imports our public API, builds a binary, and verifies it works. |
| Do we need another repo? | **No.** A CI job with `go mod edit -replace` can simulate an external module against the current checkout. A template repo is optional future convenience. |
| Does `internal/` need to change? | **No.** Everything stays in `internal/`. The public API is a thin wrapper. |
| Does the SDK need to change? | **No.** The SDK is already public and correct. |

#### Dependency Chain

```
Distributor's main.go
    │
    ├── imports: github.com/cloudblue/chaperone       (public Run() API)
    │                │
    │                └── delegates to: internal/proxy, internal/config, etc.
    │
    └── imports: github.com/cloudblue/chaperone/sdk   (public interfaces)
                     │
                     └── Plugin, Credential, TransactionContext
```

The public API is a **one-function bridge** between what Distributors can see
(the `chaperone` package) and what they cannot (the `internal/` implementation).
This is the standard Go pattern for exactly this situation.
