# Task 15: Documentation

**Status:** [ ] Not Started
**Priority:** P1
**Estimated Effort:** M

## Objective

Publish Distributor-facing documentation: Installation Guide, Configuration Reference,
Plugin Developer Guide, and Troubleshooting.

The **primary audience** is a Distributor's engineering team receiving the Chaperone
source code (pre-publication) who needs to build and deploy a custom proxy binary
with their own credential-injection plugin.

## Design Spec Reference

- **Primary:** Section 7 - Implementation Guide (Builder Pattern / "Own Repo" workflow)
- **Primary:** Section 6 - Deployment & Network Strategy
- **Primary:** Section 5.5 - Configuration Specification
- **Related:** Section 8.2 - Deployment & mTLS Enrollment
- **Related:** ADR-001 (Static Recompilation), ADR-004 (Split Modules), ADR-005 (Configurable Naming)

## Dependencies

- [x] `01-configuration.task.md` - Config structure finalized
- [x] `02-router-allowlist.task.md` - Allow-list syntax finalized
- [x] `06-resilience.task.md` - Timeout configuration finalized
- [x] `13-module-preparation.task.md` - Public API (`chaperone.Run()`) exists
- [ ] `14-enroll-public-api.task.md` - Public `Enroll()` API exists for enrollment documentation

## Key Constraints

1. **Pre-publication state:** Modules are NOT yet published on a Go proxy.
   Distributors will receive the source code as a snapshot (not a git repo).
   All documentation must describe the `replace` directive workflow for local
   module resolution, with clear notes on what changes post-publication.

2. **No `.github/` folder:** The `.github/` directory will be stripped before
   distribution. Documentation must NOT reference any file inside `.github/`.

3. **"Own Repo" is the preferred workflow.** The "Fork/Extend" workflow is
   documented as a simpler alternative but the docs should guide Distributors
   toward building their own repository.

4. **Code examples must compile.** All Go snippets must match the actual SDK
   interfaces (`sdk/plugin.go`). In particular, `ModifyResponse` returns
   `(*sdk.ResponseAction, error)`, not just `error`.

## Acceptance Criteria

### Distributor Installation Guide (`docs/installation.md`)
- [ ] Prerequisites (Go 1.25+, Docker, certificates, network access)
- [ ] Quick start with Docker (Mode A — pre-built image)
- [ ] mTLS certificate enrollment via `chaperone.Enroll()` public API
  - [ ] Document `EnrollConfig` struct and `EnrollResult` fields
  - [ ] Show wiring `enroll` subcommand in Distributor's `main.go`
  - [ ] Example: generate CSR, submit to CA, receive signed cert
- [ ] Development certificates (`make gencerts` for testing)
- [ ] Configuration file setup (copy from `configs/config.example.yaml`)
- [ ] Docker deployment operations
  - [ ] Running the container (port mapping: 8443 for proxy, 9090 for admin)
  - [ ] Volume mounts for certificates and configuration
  - [ ] Health checking without shell (distroless image has no curl/wget)
  - [ ] Kubernetes liveness/readiness probe example (`httpGet` on admin port)
  - [ ] Host-side verification: `curl -k https://localhost:9090/healthz`
  - [ ] Production hardening notes (non-root, distroless, read-only rootfs)
- [ ] Verification steps (health check, version, test proxy request)
- [ ] Troubleshooting link

### Configuration Reference (`docs/configuration.md`)
- [ ] File location resolution order (`-config` flag → `CHAPERONE_CONFIG` env → `./config.yaml`)
- [ ] All config sections documented with types, defaults, env var overrides
- [ ] Server section (addr, admin_addr, shutdown_timeout, TLS)
- [ ] Upstream section (header_prefix, trace_header, allow_list, timeouts)
- [ ] Observability section (log_level, enable_profiling, sensitive_headers)
- [ ] Allow-list glob pattern syntax (`*` vs `**`) with examples
- [ ] Environment variable override pattern (`CHAPERONE_<SECTION>_<KEY>`)
- [ ] Timeout tuning guidance (connect, read, write, idle, keep_alive, plugin)
- [ ] Complete annotated example (reference `configs/config.example.yaml`)

### Plugin Developer Guide (`docs/plugin-development.md`) — **Centerpiece**
- [ ] Architecture overview: what is a plugin, why static recompilation (ADR-001 summary)
- [ ] The `sdk.Plugin` interface: `CredentialProvider` + `CertificateSigner` + `ResponseModifier`
- [ ] TransactionContext and Credential types explained
- [ ] Fast Path vs Slow Path credential strategies with examples
- [ ] ResponseAction and when to use `SkipErrorNormalization`
- [ ] **Method 1: "Own Repo" (Recommended)**
  - [ ] Directory layout (`go.mod`, `main.go`, `plugins/`)
  - [ ] `go.mod` with `require` for both modules
  - [ ] Pre-publication note: `replace` directives for local snapshot
  - [ ] Post-publication note: remove `replace`, use versioned `require`
  - [ ] Minimal `main.go` using `chaperone.Run()` with options
  - [ ] Adding enrollment support: `chaperone.Enroll()` in subcommand routing
  - [ ] Available `chaperone.Option` values (`WithConfigPath`, `WithVersion`, `WithBuildInfo`, `WithLogOutput`)
  - [ ] Building: `go build -o my-proxy .`
  - [ ] Docker deployment: Dockerfile template for Distributor binary
  - [ ] Docker runtime: running, health checking, volume mounts, production hardening
- [ ] **Method 2: "Fork/Extend" (Simpler alternative)**
  - [ ] When to use (quick start, simple deployments)
  - [ ] Clone repo, add plugin under `plugins/`, modify `cmd/chaperone/main.go`
  - [ ] Build with `make build`
  - [ ] Tradeoffs vs Own Repo (merge conflicts on upgrade, coupled versioning)
- [ ] Testing your plugin
  - [ ] Unit testing with standard Go `testing`
  - [ ] Compliance suite: `compliance.VerifyContract(t, plugin)`
  - [ ] Integration testing tips (mock TransactionContext, use httptest)
- [ ] Common patterns: Bearer token, API key, OAuth2 refresh, Vault lookup
- [ ] Reference plugin walkthrough (`plugins/reference/reference.go`)

### Troubleshooting (`docs/troubleshooting.md`)
- [ ] Common startup errors (missing config, TLS cert errors, port conflicts)
- [ ] mTLS issues (macOS LibreSSL + ECDSA workaround, cert chain validation)
- [ ] Allow-list denials (403 responses, glob pattern debugging)
- [ ] Plugin errors (timeout, panic recovery, credential format)
- [ ] Docker-specific issues (permissions, networking, image size)
- [ ] Diagnostic tools (health endpoint, version endpoint, pprof, metrics)

### Documentation Index (`docs/README.md`)
- [ ] Overview and audience
- [ ] Quick navigation to each guide
- [ ] No references to `.github/` directory or its contents

### Root README (`README.md`)
- [ ] Add links to docs/ guides
- [ ] Ensure "Building Custom Binaries" section aligns with plugin-development.md
- [ ] Ensure Configuration Reference section aligns with docs/configuration.md
- [ ] Verify all code examples match actual SDK interfaces

### General
- [ ] All docs in `docs/` directory
- [ ] Clear, concise language targeting a Go developer audience
- [ ] All Go code examples compile against current SDK interfaces
- [ ] Copy-pasteable shell commands
- [ ] No references to files in `.github/`

## Implementation Hints

### Suggested Structure

```
docs/
├── README.md                    # Overview and navigation (update existing)
├── DESIGN-SPECIFICATION.md      # (existing — do not modify)
├── ROADMAP.md                   # (existing — do not modify)
├── installation.md              # Distributor Installation Guide
├── configuration.md             # Configuration Reference
├── plugin-development.md        # Plugin Developer Guide (centerpiece)
└── troubleshooting.md           # Common issues and solutions
```

### Plugin Developer Guide: "Own Repo" Section Outline

This is the most critical section. The Distributor's experience should be:

```
1. Receive Chaperone snapshot → extract to ~/chaperone
2. Create own repo → mkdir my-proxy && cd my-proxy && go mod init
3. Write plugin → implements sdk.Plugin
4. Write main.go → calls chaperone.Run(ctx, plugin)
5. Configure replace directives → points to local snapshot
6. Build → go build -o my-proxy .
7. Deploy → Docker or systemd
```

#### Pre-Publication go.mod Template

```go
module github.com/acme/my-proxy

go 1.25

require (
    github.com/cloudblue/chaperone     v0.0.0
    github.com/cloudblue/chaperone/sdk v0.0.0
)

// Pre-publication: point to local Chaperone source snapshot.
// Remove these directives once modules are published on a Go proxy.
replace (
    github.com/cloudblue/chaperone     => ../chaperone
    github.com/cloudblue/chaperone/sdk => ../chaperone/sdk
)
```

#### Post-Publication go.mod (what it becomes)

```go
module github.com/acme/my-proxy

go 1.25

require (
    github.com/cloudblue/chaperone     v1.5.0   // Core — upgrade freely
    github.com/cloudblue/chaperone/sdk v1.0.0   // SDK — stable interface
)
```

#### Minimal main.go (proxy only)

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/cloudblue/chaperone"
    myplugin "github.com/acme/my-proxy/plugins"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := chaperone.Run(ctx, myplugin.New(),
        chaperone.WithConfigPath("/etc/chaperone.yaml"),
        chaperone.WithVersion("1.0.0"),
    ); err != nil {
        os.Exit(1)
    }
}
```

> **Note:** For a version that also supports `./my-proxy enroll` for mTLS
> enrollment, see "Minimal main.go with Enrollment Support" below.

#### Minimal main.go with Enrollment Support

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
    if len(os.Args) > 1 && os.Args[1] == "enroll" {
        if err := runEnroll(); err != nil {
            fmt.Fprintf(os.Stderr, "enrollment failed: %v\n", err)
            os.Exit(1)
        }
        return
    }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := chaperone.Run(ctx, myplugin.New(),
        chaperone.WithConfigPath("/etc/chaperone.yaml"),
        chaperone.WithVersion("1.0.0"),
    ); err != nil {
        os.Exit(1)
    }
}

func runEnroll() error {
    result, err := chaperone.Enroll(chaperone.EnrollConfig{
        Domains:    []string{"proxy.example.com"},
        OutputDir:  "./certs",
        KeyFile:    "server.key",
        CSRFile:    "server.csr",
    })
    if err != nil {
        return err
    }
    fmt.Printf("Key:  %s\n", result.KeyPath)
    fmt.Printf("CSR:  %s\n", result.CSRPath)
    return nil
}
```

#### Distributor Dockerfile Template

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build

# Copy Chaperone snapshot (pre-publication only)
COPY chaperone/ ./chaperone/

# Copy distributor project
COPY my-proxy/ ./my-proxy/
WORKDIR /build/my-proxy

RUN CGO_ENABLED=0 GOOS=linux go build -o proxy .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/my-proxy/proxy /app/proxy
COPY config.yaml /app/config.yaml
USER nonroot:nonroot
EXPOSE 8443 9090
ENTRYPOINT ["/app/proxy"]
CMD ["-config", "/app/config.yaml"]
```

#### Docker Runtime Guide (for documentation)

The Dockerfile template above only covers _building_. Docs must also cover _running_:

```bash
# Run with certificates and config mounted
docker run -d --name chaperone-proxy \
  -p 8443:8443 \
  -p 9090:9090 \
  -v /path/to/certs:/app/certs:ro \
  -v /path/to/config.yaml:/app/config.yaml:ro \
  --read-only \
  my-proxy:latest

# Health check (from host — distroless has no shell)
curl -s http://localhost:9090/healthz
curl -s http://localhost:9090/version
```

Kubernetes probe example:
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 9090
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /healthz
    port: 9090
  initialDelaySeconds: 3
  periodSeconds: 5
```

Production hardening checklist for docs:
- Image runs as `nonroot:nonroot` (UID 65534) — already set in Dockerfile
- Uses `gcr.io/distroless/static` — no shell, minimal attack surface
- Mount certs and config as read-only (`:ro`)
- Use `--read-only` flag for read-only root filesystem
- Final image ~50MB (static Go binary + distroless base)

### Plugin Interface Quick Reference

Document these exactly as they appear in `sdk/plugin.go`:

```go
type Plugin interface {
    CredentialProvider
    CertificateSigner
    ResponseModifier
}

type CredentialProvider interface {
    GetCredentials(ctx context.Context, tx TransactionContext, req *http.Request) (*Credential, error)
}

type CertificateSigner interface {
    SignCSR(ctx context.Context, csrPEM []byte) (crtPEM []byte, err error)
}

type ResponseModifier interface {
    ModifyResponse(ctx context.Context, tx TransactionContext, resp *http.Response) (*ResponseAction, error)
}
```

Note: `ModifyResponse` returns `(*ResponseAction, error)`, NOT just `error`.
The `ResponseAction` type has a `SkipErrorNormalization` field.

### Configuration Reference: Avoid Duplication with README.md

The root `README.md` already contains a comprehensive configuration reference.
The `docs/configuration.md` should be the **authoritative, complete** reference.
After creating it, consider whether to slim down the README's config section
to a summary with a link to `docs/configuration.md`, or keep both in sync.

Recommendation: Keep README as quick-start summary, `docs/configuration.md` as
the full reference with timeout tuning guidance and operational recommendations.

### Key Source Files to Reference

These files contain the actual implementations the docs describe:

| Topic | Source File |
|-------|------------|
| Plugin interface | `sdk/plugin.go` |
| TransactionContext | `sdk/context.go` |
| Credential type | `sdk/credential.go` |
| ResponseAction | `sdk/plugin.go` (bottom) |
| Compliance suite | `sdk/compliance/verify.go` |
| Public API (Run) | `chaperone.go` |
| Public API (Enroll) | `chaperone.go` (EnrollConfig, EnrollResult, Enroll) |
| Option types | `chaperone.go` |
| Reference plugin | `plugins/reference/reference.go` |
| Example config | `configs/config.example.yaml` |
| CLI entry point | `cmd/chaperone/main.go` |
| Dockerfile | `Dockerfile` |

### Gotchas

- **Interface accuracy:** Always reference `sdk/plugin.go` as the source of truth
  for interface signatures. The Design Spec §7 has been updated to match the SDK
  but if any drift occurs, the code wins.
- **Keep docs in sync with code** — version together
- **Test all code examples** — ensure they compile against current SDK
- **Include copy-pasteable commands** — no manual interpolation needed
- **Address common mistakes proactively** — especially `replace` directive syntax
- **Link between docs** for navigation
- **No `.github/` references** — this folder is stripped before distribution

## Files to Create/Modify

### Create
- [ ] `docs/installation.md` - Installation guide
- [ ] `docs/configuration.md` - Configuration reference
- [ ] `docs/plugin-development.md` - Plugin developer guide
- [ ] `docs/troubleshooting.md` - Troubleshooting

### Modify
- [ ] `docs/README.md` - Update to be a documentation index (remove `.github/` references)
- [ ] `README.md` - Add links to `docs/` guides, verify code examples match SDK

## Testing Strategy

- **Review:**
  - Technical accuracy review against `sdk/plugin.go` and `chaperone.go`
  - Fresh-eyes walkthrough (someone unfamiliar with Chaperone)
- **Validation:**
  - Follow "Own Repo" instructions to create a new project from scratch
  - Verify all Go code examples compile (`go build`)
  - Run `scripts/test-distributor-workflow.sh` as sanity check
  - Test all curl commands against a running instance
  - Verify `replace` directive workflow with local snapshot layout
  - Confirm no references to `.github/` in any documentation file
