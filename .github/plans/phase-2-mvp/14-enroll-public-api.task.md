# Task 14: Enroll Public API

**Status:** [x] Completed
**Priority:** P1
**Estimated Effort:** S

## Objective

Expose the `enroll` (CSR generation) functionality as a public API in the
`chaperone` package so that Distributors building custom binaries via the
"Own Repo" workflow get the `enroll` subcommand for free.

## Problem Statement

The `enroll` subcommand is an essential production tool — it generates the
ECDSA P-256 key pair and CSR that Distributors submit to their CA to obtain
a signed server certificate for mTLS (Design Spec §8.2).

Currently, `enroll` lives entirely in `cmd/chaperone/enroll.go` and imports
`internal/cli` (for domain flag parsing). This means:

| Workflow | `enroll` available? | Why |
|----------|---------------------|-----|
| **Fork/Extend** (clone this repo, build `cmd/chaperone`) | ✅ Yes | Same module, `internal/` is accessible |
| **Own Repo** (recommended — Distributor's own `main.go` + `chaperone.Run()`) | ❌ No | `internal/cli` is not importable externally |

Since "Own Repo" is the **recommended production workflow** (Design Spec §7),
Distributors would be unable to generate production certificates from their
own binary. They'd need to either:
- Build *our* `cmd/chaperone` binary separately just for enrollment (confusing)
- Manually generate CSRs with OpenSSL (error-prone, loses the nice UX)

Neither is acceptable for an MVP aimed at Early Adopter Distributors.

## Design Spec Reference

- **Primary:** Section 8.2 - Deployment & mTLS Enrollment
- **Related:** Section 7 - Implementation Guide (Builder Pattern)
- **Related:** ADR-004 - Split Module Versioning

## Dependencies

- [x] `13-module-preparation.task.md` - Public API (`chaperone.Run()`) exists
- `pkg/crypto` already provides `GenerateServerCSR()` (public, no changes needed)

## Acceptance Criteria

### Public API
- [x] `chaperone.Enroll()` function exists in the `chaperone` package
- [x] Generates ECDSA P-256 key pair and CSR (delegates to `pkg/crypto.GenerateServerCSR`)
- [x] Writes `server.key` and `server.csr` to the specified output directory
- [x] Returns structured results (file paths written, SANs) — not just printing to stdout
- [x] Returns errors instead of calling `os.Exit` (library-friendly)
- [x] Domain/IP parsing inlined or moved to a public location (currently in `internal/cli`)

### CLI Integration
- [x] `cmd/chaperone/enroll.go` refactored to call `chaperone.Enroll()`
- [x] All existing CLI behavior preserved (flags, usage text, exit codes)
- [x] Existing tests still pass

### Distributor Enablement
- [x] A Distributor's `main.go` can wire up `enroll` with ~5 lines of code
- [x] Example in godoc shows how to add enroll support

## Implementation Plan

### Step 1: Design the Public API

Add to `chaperone.go` (or a new `enroll.go` at module root):

```go
// EnrollConfig configures CSR generation for production CA enrollment.
type EnrollConfig struct {
    // Domains is a comma-separated list of DNS names and IP addresses
    // for the server certificate's Subject Alternative Names.
    // Example: "proxy.example.com,10.0.0.1"
    Domains string

    // CommonName is the certificate's Common Name field.
    // Default: "chaperone"
    CommonName string

    // OutputDir is the directory where server.key and server.csr are written.
    // The directory is created if it does not exist.
    // Default: "certs"
    OutputDir string
}

// EnrollResult contains the output of a successful enrollment.
type EnrollResult struct {
    KeyFile  string   // Path to the generated private key
    CSRFile  string   // Path to the generated CSR
    DNSNames []string // DNS SANs included in the CSR
    IPs      []net.IP // IP SANs included in the CSR
}

// Enroll generates an ECDSA P-256 key pair and Certificate Signing Request
// for production CA enrollment. The CSR can be submitted to a CA (Connect
// Portal, HashiCorp Vault, internal PKI, etc.) to obtain a signed server
// certificate for mTLS.
//
// This is the programmatic equivalent of `chaperone enroll --domains ...`.
//
// See Design Spec Section 8.2 for the full enrollment workflow.
func Enroll(cfg EnrollConfig) (*EnrollResult, error) {
    // 1. Validate and parse Domains into dnsNames + ips
    //    (inline the logic from internal/cli.ParseDomainsFlag)
    // 2. Create OutputDir if needed
    // 3. Call pkg/crypto.GenerateServerCSR(commonName, dnsNames, ips)
    // 4. Write server.key and server.csr with 0600 permissions
    // 5. Return EnrollResult
}
```

### Step 2: Move Domain Parsing

The `internal/cli.ParseDomainsFlag` function is 15 lines. Two options:

**Option A (Preferred): Inline into `Enroll()`.**
The parsing logic is trivial (split by comma, `net.ParseIP` each entry).
Inlining avoids creating a new public package for a single tiny helper.
The `internal/cli` package continues to exist for `cmd/chaperone` flag parsing.

**Option B: Move to `pkg/cli`.**
Makes it importable but creates a public package with a single function.
Unnecessary if we inline.

Recommendation: **Option A.** Create a private `parseCSRDomains` helper in
the `chaperone` package. `internal/cli.ParseDomainsFlag` stays where it is
(used by `cmd/chaperone` flag handling).

### Step 3: Refactor `cmd/chaperone/enroll.go`

After the public API exists, `cmd/chaperone/enroll.go` becomes a thin CLI
wrapper:

```go
func enrollCmd(args []string) {
    // Parse flags (--domains, --cn, --out) — unchanged
    // ...

    result, err := chaperone.Enroll(chaperone.EnrollConfig{
        Domains:    *domainsFlag,
        CommonName: *commonName,
        OutputDir:  *outputDir,
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }

    // Print results — unchanged
    printEnrollmentInstructions(result)
}
```

### Step 4: Distributor Usage Example

A Distributor adding `enroll` to their binary:

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/cloudblue/chaperone"
    myplugin "github.com/acme/my-proxy/plugins"
)

func main() {
    // Handle enroll subcommand
    if len(os.Args) > 1 && os.Args[1] == "enroll" {
        result, err := chaperone.Enroll(chaperone.EnrollConfig{
            Domains: os.Args[2], // simplified — use flag package for production
        })
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error: %v\n", err)
            os.Exit(1)
        }
        fmt.Printf("CSR written to %s\n", result.CSRFile)
        fmt.Printf("Key written to %s\n", result.KeyFile)
        return
    }

    // Normal proxy startup
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := chaperone.Run(ctx, myplugin.New()); err != nil {
        os.Exit(1)
    }
}
```

## Key Source Files

| File | Role |
|------|------|
| `chaperone.go` (or new `enroll.go` at root) | New `Enroll()` public function |
| `cmd/chaperone/enroll.go` | Refactor to delegate to `chaperone.Enroll()` |
| `internal/cli/flags.go` | Stays as-is (used by cmd CLI flag parsing) |
| `pkg/crypto/certs.go` | Stays as-is (`GenerateServerCSR` already public) |

## Files to Create/Modify

### Create
- [x] `enroll.go` (at module root) — `Enroll()`, `EnrollConfig`, `EnrollResult`, `parseCSRDomains`
- [x] `enroll_test.go` (at module root) — tests for `Enroll()` function

### Modify
- [x] `cmd/chaperone/enroll.go` — refactor to delegate to `chaperone.Enroll()`

### Unchanged
- `internal/cli/flags.go` — stays as-is
- `internal/cli/flags_test.go` — stays as-is
- `pkg/crypto/certs.go` — stays as-is (already public)

## Testing Strategy

- [x] `enroll_test.go`: `TestEnroll_WritesKeyAndCSR` — verify files written with correct permissions
- [x] `enroll_test.go`: `TestEnroll_ParsesDNSAndIPs` — verify mixed domain/IP parsing
- [x] `enroll_test.go`: `TestEnroll_EmptyDomains_ReturnsError` — error on empty input
- [x] `enroll_test.go`: `TestEnroll_CreatesOutputDir` — creates missing directory
- [x] `enroll_test.go`: `TestEnroll_DefaultValues` — CommonName defaults to "chaperone", OutputDir to "certs"
- [x] Verify `cmd/chaperone/enroll.go` still passes existing integration tests
- [x] Verify `scripts/test-distributor-workflow.sh` still passes

## Relationship to Task 15 (Documentation)

Task 15 documents the "Own Repo" workflow for Distributors. Once this task
is complete, the Plugin Developer Guide (`docs/plugin-development.md`) should
document `chaperone.Enroll()` alongside `chaperone.Run()` as part of the
public API, including how to wire up the `enroll` subcommand in a Distributor's
`main.go`. This unblocks documenting a complete single-binary workflow.

**Recommended execution order:** This task (14) before Task 15, so the docs
can reference the finalized public API.
