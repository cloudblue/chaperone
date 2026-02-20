# Chaperone - Copilot Instructions

This document provides context and coding guidelines for GitHub Copilot when working on the Chaperone project.

> **Quick Start:** For new sessions, read `AGENTS.md` at project root first.

## Project Overview

**Chaperone** is a high-performance, sidecar-style egress proxy designed to inject sensitive credentials into outgoing API requests. It runs within a Distributor's infrastructure and ensures credentials never leak to upstream platforms or logs.

### Key Characteristics

- **Language:** Go (Golang) - chosen for concurrency, static binaries, and robust stdlib
- **Architecture:** Plugin-based via static recompilation ("Caddy Model" - see ADR-001)
- **License:** Apache 2.0
- **Primary Concern:** Security first, then performance

### Architecture Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Proxy Core | `internal/` | mTLS termination, routing, caching, response sanitization |
| Plugin SDK | `sdk/` | Public interfaces (separate Go module) that Distributors implement |
| Reference Plugin | `plugins/reference/` | Default plugin for testing and simple deployments |
| CLI Entry | `cmd/chaperone/` | Main binary entry point |

## Project Structure (Multi-Module Monorepo)

This repository contains **two Go modules** for independent versioning (ADR-004):

```
/chaperone
├── go.mod                  # module github.com/cloudblue/chaperone (Core)
├── cmd/chaperone/          # Main entry point → builds working proxy
│   └── main.go
│
├── sdk/                    # SEPARATE MODULE: github.com/cloudblue/chaperone/sdk
│   ├── go.mod              # Independent versioning (tagged as sdk/v1.x.x)
│   ├── plugin.go           # Plugin interface definitions
│   ├── context.go          # TransactionContext struct
│   ├── credential.go       # Credential struct
│   └── compliance/         # Test kit for Distributors
│       └── verify.go
│
├── internal/               # Private implementation (not importable)
│   ├── proxy/              # Core reverse proxy logic
│   ├── config/             # YAML + env var configuration
│   ├── cache/              # memguard-based credential cache
│   └── observability/      # Metrics, logging, tracing
│
├── plugins/                # Built-in plugin implementations
│   └── reference/          # Default file-based credential provider
│
├── chaperone.go            # Public API: Run(), types
├── configs/                # Example configuration files
├── deployments/            # Docker, K8s, Systemd files
└── test/                   # Integration tests
```

### Module Versioning Strategy

| Module | Path | Tagging | Purpose |
|--------|------|---------|---------|
| Core | `github.com/cloudblue/chaperone` | `v1.x.x` | Proxy engine, internal logic |
| SDK | `github.com/cloudblue/chaperone/sdk` | `sdk/v1.x.x` | Public interfaces (stable API) |

**Release workflow:**
```bash
# Tag SDK (rarely changes)
git tag sdk/v1.0.0

# Tag Core (frequent updates)
git tag v1.5.0
```

Distributors can upgrade Core without touching their plugin code:
```go
require (
    github.com/cloudblue/chaperone/sdk v1.0.0  // Stable interface
    github.com/cloudblue/chaperone v1.5.0      // Can upgrade freely
)
```

## Go Coding Standards

### General Conventions

- Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` and `goimports` for formatting
- Package names: short, lowercase, no underscores (e.g., `proxy`, `config`)
- File names: lowercase with underscores (e.g., `reverse_proxy.go`, `cache_test.go`)

### Error Handling

**Always return errors with context wrapping.** Never panic for recoverable errors.

```go
// ✅ Correct: Wrap errors with context
if err != nil {
    return fmt.Errorf("credential fetch for vendor %s failed: %w", vendorID, err)
}

// ✅ Correct: Sentinel errors for specific conditions
var ErrCredentialExpired = errors.New("credential expired")

// ❌ Wrong: Panic for runtime errors
if err != nil {
    panic(err)  // Never do this
}

// ❌ Wrong: Swallowing errors
result, _ := someFunction()  // Never ignore errors
```

### Logging

Use `log/slog` (standard library) for structured logging:

```go
slog.Info("request processed",
    "trace_id", ctx.TraceID,
    "vendor_id", ctx.VendorID,
    "latency_ms", elapsed.Milliseconds(),
    "status", resp.StatusCode)

slog.Error("upstream connection failed",
    "error", err,
    "target_host", targetURL.Host)
```

### Concurrency

- Prefer channels for communication, mutexes for state protection
- Always use `context.Context` for cancellation and timeouts
- Document goroutine ownership clearly

## Security Requirements

### Mandatory Security Practices

1. **Never log credentials** - Headers like `Authorization`, `Cookie`, `X-API-Key` must be redacted
2. **Use `memguard`** for in-memory credential storage (encrypted, guarded pages)
3. **Validate all inputs** - Especially `X-Connect-Target-URL` against allow-list
4. **Strip credentials from responses** - Sanitize before returning to upstream

### Secure Defaults

When suggesting code, always prefer secure defaults even if slightly slower:

| Feature | Secure Default | Performance Impact |
|---------|---------------|-------------------|
| TLS Version | TLS 1.3 minimum | Negligible |
| Certificate Validation | Always verify | ~1-2ms handshake |
| Memory Protection | `memguard` for secrets | ~50μs per access |
| Timeouts | Strict defaults | Prevents resource exhaustion |

When a less secure but faster option exists, mention it with explicit security tradeoffs.

### Redaction List

These headers must ALWAYS be redacted in logs:

```go
var sensitiveHeaders = []string{
    "Authorization",
    "Proxy-Authorization", 
    "Cookie",
    "Set-Cookie",
    "X-API-Key",
    "X-Auth-Token",
}
```

## Testing Requirements

### Test-Driven Development (TDD)

Follow TDD workflow:
1. Write a failing test first
2. Implement minimal code to pass
3. Refactor while keeping tests green

### Test Structure

```go
func TestFunctionName_Scenario_ExpectedBehavior(t *testing.T) {
    // Arrange
    input := ...
    expected := ...
    
    // Act
    result, err := FunctionUnderTest(input)
    
    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != expected {
        t.Errorf("got %v, want %v", result, expected)
    }
}
```

### Test Categories

| Type | Location | Purpose |
|------|----------|---------|
| Unit | `*_test.go` alongside code | Core logic, parsing, hashing |
| Integration | `test/integration/` | mTLS handshake, plugin contract |
| Compliance | `pkg/sdk/compliance/` | Plugin contract verification |
| Fuzz | `*_fuzz_test.go` | Input parsing, config loading |

### Table-Driven Tests

Prefer table-driven tests for multiple scenarios:

```go
func TestValidateTargetURL(t *testing.T) {
    tests := []struct {
        name      string
        url       string
        allowList map[string][]string
        wantErr   bool
    }{
        {"valid exact match", "https://api.vendor.com/v1", ..., false},
        {"blocked host", "https://evil.com/data", ..., true},
        {"path not allowed", "https://api.vendor.com/admin", ..., true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateTargetURL(tt.url, tt.allowList)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateTargetURL() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## Configuration

### Config File Format

YAML configuration with environment variable overrides:

```yaml
server:
  addr: ":443"
  admin_addr: ":9090"

upstream:
  header_prefix: "X-Connect"
  allow_list:
    "api.vendor.com":
      - "/v1/**"
```

### Environment Variables

Pattern: `CHAPERONE_<SECTION>_<KEY>` (uppercase, underscore separator)

Example: `CHAPERONE_SERVER_ADDR=":8443"`

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `security`

Examples:
- `feat(proxy): add credential caching with TTL support`
- `fix(config): handle missing allow_list gracefully`
- `security(sanitizer): redact Authorization header from error responses`
- `test(sdk): add compliance suite for CredentialProvider`

## Architecture Decision Records

Reference these ADRs from `docs/explanation/DESIGN-SPECIFICATION.md` when making architectural choices:

| ADR | Decision | Rationale |
|-----|----------|-----------|
| ADR-001 | Static Recompilation for plugins | Performance, capability, reliability |
| ADR-002 | Go as language | Concurrency, deployment, stdlib |
| ADR-003 | Hybrid caching (Fast/Slow path) | Balance performance and flexibility |
| ADR-004 | Split modules (SDK vs Core) | Independent versioning |
| ADR-005 | Configurable naming | Platform-agnostic design |

## Dependencies Policy

**"Standard Library First"** - Minimize external dependencies to reduce supply chain risk.

### Approved External Dependencies

| Package | Purpose | Justification |
|---------|---------|---------------|
| `github.com/awnumar/memguard` | Secure memory | Critical for credential protection |
| `gopkg.in/yaml.v3` | YAML parsing | Config file format |
| `github.com/prometheus/client_golang` | Metrics | Industry standard observability |

### Adding New Dependencies

Before adding a dependency:
1. Check if stdlib provides the functionality
2. Evaluate security posture (maintainers, CVE history)
3. Consider vendoring for critical security code
4. Document justification in PR

## Code Generation

When generating code for Chaperone:

1. **Check the Design Spec first** - `docs/explanation/DESIGN-SPECIFICATION.md` is the source of truth
2. **Follow the Plugin interface** - Changes to `pkg/sdk/` affect all Distributors
3. **Include tests** - Every new function needs corresponding test
4. **Add doc comments** - Exported functions need godoc comments
5. **Consider backward compatibility** - SDK changes need careful versioning

## File Header

All Go files should include:

```go
// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0
```
