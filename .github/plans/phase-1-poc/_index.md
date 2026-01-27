# Phase 1: Proof of Concept (PoC)

**Goal:** Validate the "Static Recompilation" architecture, mTLS handshake, and Context Logic using a Docker-only environment.

**Status:** In Progress

## Task Overview

| # | Task | Status | Priority | Dependencies | Est. Effort |
|---|------|--------|----------|--------------|-------------|
| 01 | SDK Module Setup | [x] | P0 | - | S |
| 02 | TransactionContext Struct | [x] | P0 | - | S |
| 03 | Credential Struct | [x] | P0 | - | S |
| 04 | Plugin Interfaces | [x] | P0 | 02, 03 | S |
| 05 | Context Parsing | [x] | P0 | 02 | M |
| 06 | Context Hashing | [x] | P1 | 02 | M |
| 07 | Reference Plugin | [x] | P0 | 04 | M |
| 08 | Core Skeleton | [ ] | P0 | 05, 07 | L |
| 09 | Plugin Mechanism Verification | [ ] | P0 | 07, 08 | S |
| 10 | mTLS Verification | [ ] | P0 | 08 | L |
| 11 | Docker Validation | [ ] | P0 | All | M |

**Legend:** `[ ]` Not started | `[~]` In progress / Pending Review | `[x]` Completed | `[!]` Blocked

## Dependency Graph

```
Tier 0 (Foundations) - Already Done ✓
┌─────────────────────────────────────────────────────────────────┐
│  01-sdk-module [x]                                              │
│  02-transaction-context [x]                                     │
│  03-credential-struct [x]                                       │
│  04-plugin-interfaces [x]                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
Tier 1 (Core Logic)
┌─────────────────────────────────────────────────────────────────┐
│  05-context-parsing ──┐                                         │
│  06-context-hashing   │                                         │
│  07-reference-plugin ─┴──────────────────────┐                  │
└──────────────────────────────────────────────│──────────────────┘
                                               │
                              ┌────────────────┘
                              ▼
Tier 2 (HTTP Layer)
┌─────────────────────────────────────────────────────────────────┐
│  08-core-skeleton                                               │
│       └── 09-plugin-mechanism                                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
Tier 3 (Security)
┌─────────────────────────────────────────────────────────────────┐
│  10-mtls-verification                                           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
Tier 4 (Deployment)
┌─────────────────────────────────────────────────────────────────┐
│  11-docker-validation                                           │
└─────────────────────────────────────────────────────────────────┘
```

## Progress Notes

### Completed

- **01-04:** SDK module structure exists with interfaces and structs defined in `sdk/`.
- Reference plugin partially exists in `plugins/reference/`.

### Current Focus

- **05-context-parsing:** Implement header parsing into TransactionContext
- **07-reference-plugin:** Complete implementation with all interfaces

### Blockers

None currently.

## Phase Exit Criteria

- [ ] All tasks marked `[x]`
- [ ] `go build ./...` succeeds
- [ ] `make test` passes (all modules)
- [ ] `make lint` passes
- [ ] Docker image builds and runs
- [ ] mTLS handshake verified via httptest
