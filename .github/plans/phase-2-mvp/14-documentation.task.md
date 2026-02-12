# Task 14: Documentation

**Status:** [ ] Not Started
**Priority:** P1
**Estimated Effort:** M

## Objective

Publish Distributor Installation Guide, Configuration Reference, and basic Plugin Developer documentation.

## Design Spec Reference

- **Primary:** Section 6 - Deployment & Network Strategy
- **Primary:** Section 7 - Implementation Guide (Builder Pattern)
- **Primary:** Section 5.5 - Configuration Specification
- **Related:** Section 8.2 - Deployment & mTLS Enrollment

## Dependencies

- [x] `01-configuration.task.md` - Config structure must be finalized
- [x] `02-router-allowlist.task.md` - Allow-list syntax documented
- [x] `06-resilience.task.md` - Timeout configuration documented
- [ ] `13-module-preparation.task.md` - Public API must exist for Plugin Developer Guide
- [ ] All implementation tasks substantially complete

## Acceptance Criteria

### Distributor Installation Guide
- [ ] Prerequisites (Docker, Go, certificates)
- [ ] Quick start with Docker (Mode A)
- [ ] mTLS certificate enrollment (`enroll` command)
- [ ] Configuration file setup
- [ ] Verification steps (health check, test request)
- [ ] Troubleshooting common issues

### Configuration Reference
- [ ] All config sections documented with examples
- [ ] Environment variable override syntax
- [ ] Allow-list glob pattern syntax with examples
- [ ] Timeout tuning guidance
- [ ] Sensitive headers configuration
- [ ] Complete example `config.yaml`

### Plugin Developer Guide (Basic)
- [ ] Plugin interface overview
- [ ] Reference plugin walkthrough
- [ ] Building a custom binary
- [ ] Testing with compliance suite
- [ ] Common patterns (Vault, OAuth, API keys)

### General
- [ ] All docs in `docs/` directory
- [ ] Clear, concise language
- [ ] Code examples tested and working
- [ ] Links to Design Spec for deeper details

## Implementation Hints

### Suggested Structure

```
docs/
├── README.md                    # Overview and navigation
├── installation.md              # Distributor Installation Guide
├── configuration.md             # Configuration Reference
├── plugin-development.md        # Plugin Developer Guide
└── troubleshooting.md           # Common issues and solutions
```

### Installation Guide Outline

```markdown
# Distributor Installation Guide

## Prerequisites
- Docker 20.10+ OR Go 1.21+
- mTLS certificates (server cert, key, CA cert)
- Network access to ISV APIs

## Quick Start (Docker)
1. Pull or build image
2. Generate certificates: `./chaperone enroll --domains proxy.example.com`
3. Create config.yaml
4. Run: `docker run -p 443:443 -v ./certs:/certs -v ./config.yaml:/config.yaml chaperone:latest`

## Verification
- Health check: `curl -k https://localhost/_ops/health`
- Test request: (example with mTLS)

## Production Deployment
- Kubernetes reference manifests
- Systemd service file
- Security hardening checklist
```

### Configuration Reference Outline

```markdown
# Configuration Reference

## File Location
Default: `./config.yaml`
Override: `CHAPERONE_CONFIG=/path/to/config.yaml`

## Server Section
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| addr | string | ":443" | Traffic port |
| admin_addr | string | ":9090" | Metrics/admin port |

## Upstream Section
### Allow-List Patterns
- `*` - Single level wildcard
- `**` - Recursive wildcard
- Examples...

## Environment Variable Overrides
Pattern: `CHAPERONE_<SECTION>_<KEY>`
Example: `CHAPERONE_SERVER_ADDR=":8443"`
```

### Key Code Locations

- `docs/installation.md` - Installation guide
- `docs/configuration.md` - Config reference
- `docs/plugin-development.md` - Plugin guide
- `docs/troubleshooting.md` - Troubleshooting
- `configs/config.example.yaml` - Example config (created in task 01)

### Gotchas

- Keep docs in sync with code (version together)
- Test all code examples before publishing
- Include copy-pasteable commands
- Address common mistakes proactively
- Link between docs for navigation

## Files to Create/Modify

- [ ] `docs/README.md` - Documentation index
- [ ] `docs/installation.md` - Installation guide
- [ ] `docs/configuration.md` - Config reference
- [ ] `docs/plugin-development.md` - Plugin guide
- [ ] `docs/troubleshooting.md` - Troubleshooting
- [ ] `README.md` - Update project README with doc links

## Testing Strategy

- **Review:** 
  - Technical accuracy review
  - Fresh-eyes walkthrough (someone unfamiliar)
- **Validation:**
  - Follow installation guide on clean machine
  - Verify all code examples compile/run
  - Test all curl commands
