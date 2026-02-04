# Phase 2: Minimum Viable Product (MVP)

**Goal:** A secure, distributable version that Early Adopter Distributors can deploy in "Mode A".

**Status:** In Progress

## Task Overview

| # | Task | Status | Priority | Dependencies | Est. Effort |
|---|------|--------|----------|--------------|-------------|
| 01 | Configuration | [x] | P0 | Phase 1 | M |
| 02 | Router (Allow-List) | [x] | P0 | 01 | M |
| 03 | Error Normalization | [ ] | P0 | 01 | M |
| 04 | Security Layer | [ ] | P0 | 01 | M |
| 05 | Observability (Logs) | [ ] | P0 | 01, 04 | M |
| 06 | Resilience | [ ] | P0 | 01 | L |
| 07 | Telemetry (Metrics) | [ ] | P1 | Phase 1 only | M |
| 08 | Telemetry (Tracing) | [ ] | P1 | 07 | L |
| 09 | Profiling | [~] | P1 | Phase 1 only | M |
| 10 | Performance Attribution | [ ] | P1 | Phase 1 only | M |
| 11 | Benchmark Testing | [ ] | P1 | 09 | M |
| 12 | Load Testing | [ ] | P1 | 07, 11 | M |
| 13 | Documentation | [ ] | P1 | 01-06 | M |
| 14 | Module Preparation | [ ] | P1 | All | S |

**Legend:** `[ ]` Not started | `[~]` In progress | `[x]` Completed | `[!]` Blocked

## Parallel Workstreams

This phase supports **two parallel workstreams** for sprint efficiency:

### Workstream A: Core Features (Tasks 01-06, 13-14)
Sequential work on configuration, security, and resilience.

```
01-configuration
    ├── 02-router-allowlist
    ├── 03-error-normalization
    ├── 04-security-layer
    │       └── 05-observability-logs
    └── 06-resilience
            └── 13-documentation
                    └── 14-module-preparation
```

### Workstream B: Telemetry & Performance (Tasks 07-12)
**Independent** - can run in parallel with Workstream A.

```
Phase 1 (Complete)
    ├── 07-telemetry-metrics ────────────────┐
    │       └── 08-telemetry-tracing         │
    │                                        │
    ├── 09-profiling [IN PROGRESS]           │
    │       └── 11-benchmark-testing         │
    │                                        │
    └── 10-performance-attribution           │
                                             │
            ┌────────────────────────────────┘
            ▼
    12-load-testing (benefits from 07 + 11)
```

## Dependency Graph

```
Phase 1 (Complete) ─────────────────────────────────────────────────────────────┐
                                                                                │
┌───────────────────────────────────────────────────────────────────────────────┼───┐
│ Workstream A (Core Features)                                                  │   │
│                                                                               │   │
│   01-configuration ─────┬─────────────────────────────────────────────┐       │   │
│           │             │                                             │       │   │
│           ▼             ▼                                             ▼       │   │
│   02-router      03-error-norm                                 06-resilience  │   │
│                         │                                             │       │   │
│                  04-security-layer                                    │       │   │
│                         │                                             │       │   │
│                         ▼                                             │       │   │
│                  05-observability-logs                                │       │   │
│                         │                                             │       │   │
│                         └──────────────────┬──────────────────────────┘       │   │
│                                            ▼                                  │   │
│                                     13-documentation                          │   │
│                                            │                                  │   │
│                                            ▼                                  │   │
│                                   14-module-preparation                       │   │
│                                                                               │   │
└───────────────────────────────────────────────────────────────────────────────┘   │
                                                                                    │
┌───────────────────────────────────────────────────────────────────────────────────┘
│ Workstream B (Telemetry & Performance) - Independent from Workstream A
│
│   ┌─────────────────┬─────────────────────┬─────────────────────┐
│   │                 │                     │                     │
│   ▼                 ▼                     ▼                     │
│   07-metrics    09-profiling [~]    10-perf-attribution         │
│   │                 │                                           │
│   ▼                 ▼                                           │
│   08-tracing    11-benchmarks                                   │
│   │                 │                                           │
│   └────────┬────────┘                                           │
│            ▼                                                    │
│       12-load-testing                                           │
│                                                                 │
└─────────────────────────────────────────────────────────────────────────────────────
```

## Design Spec Coverage

| Task | Primary Design Spec Sections |
|------|------------------------------|
| 01 | §5.5.A Config Structure, §5.5.B Env Overrides, ADR-005 |
| 02 | §5.3 Security Controls (Allow-List), §5.5.A allow_list |
| 03 | §5.3 Error Masking, §5.2.B Response Handling |
| 04 | §5.3 Redaction & Reflection, §8.3.5 Log Privacy |
| 05 | §8.3.5 Structured Logs, §8.3.1 Trace Correlation |
| 06 | §8.1 Resilience & Reliability |
| 07 | §8.3.2 Metrics, §5.1.C Admin Endpoints |
| 08 | §8.3.1 Distributed Tracing, §8.3 OTel Note |
| 09 | §5.1.C Admin Endpoints (`/debug/pprof`), §9.3.C Profiling |
| 10 | §9.3.D Performance Attribution (Server-Timing) |
| 11 | §9.3.A Benchmark Testing |
| 12 | §9.3.B Load Testing (k6) |
| 13 | §6 Deployment, §7 Implementation Guide, §5.5 Config |
| 14 | ADR-004 Split Modules, §5.4 Versioning |

## Sprint Planning Notes

**Sprint Duration:** 2 weeks

**Recommended Allocation:**

| Week | Workstream A | Workstream B |
|------|--------------|--------------|
| 1 | Tasks 01, 02, 03 | Tasks 07, 09 (profiling in progress), 10 |
| 2 | Tasks 04, 05, 06 | Tasks 08, 11, 12 |
| Final | Tasks 13, 14 (joint) | Support |

## Progress Notes

*(To be updated during implementation)*

### Completed

*(None yet)*

### In Progress

- **Task 09 (Profiling)** - Started by coworker

### Blockers

*(None yet)*

## Phase Exit Criteria

- [ ] All tasks marked `[x]`
- [ ] `go build ./...` succeeds
- [ ] `make test` passes (all modules)
- [ ] `make lint` passes
- [ ] `make bench` passes with no regressions
- [ ] Config loading verified with example file
- [ ] Allow-list validation working
- [ ] Metrics endpoint accessible (`/metrics`)
- [ ] Profiling endpoint accessible (`/debug/pprof`)
- [ ] Server-Timing header present in responses
- [ ] Benchmark baseline established
- [ ] Documentation reviewed and complete
- [ ] No `replace` directives in go.mod files
