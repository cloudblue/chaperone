# Project Roadmap & Delivery Stages

We define three key milestones to validate the architecture and reach production readiness.

## Phase 1: Proof of Concept (PoC) ✅

**Goal:** Validate the "Static Recompilation" architecture, mTLS Handshake, and Context Logic using a **Docker-only environment**.

* [x] **Core Skeleton:** Implement a basic Go HTTP Server using `httputil.ReverseProxy`.
* [x] **Context Parsing:** Implement the `TransactionContext` extractor for `X-Connect-*` headers.
* [x] **Context Hashing:** Implement the deterministic hashing logic (canonicalization) of the Context to validate the caching strategy inputs.
* [x] **Reference Plugin:** Build the `ReferenceProvider` that reads JSON file credentials (file-based auth).
* [x] **Plugin Mechanism:** Verify the compiler can build a single binary from Core + Plugin sources (ADR-001).
* [x] **mTLS Server (Mode A):** Integrate mTLS into the HTTP server with TLS 1.3 minimum. Verify via `httptest` with client certificates.
* [x] **Docker Validation:** Verify the PoC compiles and runs inside a multi-stage Dockerfile with distroless base.

## Phase 2: Minimum Viable Product (MVP)

**Goal:** A secure, distributable version that Early Adopter Distributors can deploy in "Mode A".

### Core Features
* [ ] **Configuration:** Implement `config.yaml` loading with Environment Variable overrides.
* [ ] **Router (Allow-List):** Validate `Target-URL` host and path against configurable allow-list, enforcing "Default Deny".
* [ ] **Error Normalization:** Intercept upstream `400/500` errors, return sanitized JSON responses, hide stack traces.
* [ ] **Security Layer:** Implement the "Redactor" middleware (for logs) and "Reflector" protection (stripping Auth headers from responses).
* [ ] **Observability (Logs):** Implement structured JSON logging to `STDOUT` (Trace ID, Status, Latency) with header redaction.
* [ ] **Resilience:** Configurable timeouts (Read/Write/Idle), graceful shutdown, panic recovery middleware.

### Telemetry
* [ ] **Telemetry (Metrics):** Expose `/metrics` endpoint with Prometheus counters and histograms.
* [ ] **Telemetry (Tracing):** OpenTelemetry integration with OTLP exporters for distributed tracing.

### Performance
* [ ] **Profiling:** Add the optional `/debug/pprof` endpoint (protected by config).
* [ ] **Performance Attribution:** Implement the `Server-Timing` header logic to visualize "Plugin vs. Upstream" latency.
* [ ] **Benchmark Testing:** Go native benchmarks for hot path components, memory efficiency, and TLS performance.
* [x] **Load Testing:** k6 scripts for stress testing with defined scenarios and success criteria.

### Finalization
* [ ] **Documentation:** Publish Distributor Installation Guide, Configuration Reference, and basic Plugin Developer docs.
* [ ] **Module Preparation:** Remove `replace` directives, verify independent module imports, prepare for future publication.

## Phase 3: Production Ready (V1.0)

**Goal:** Operational excellence, advanced features, and enterprise-grade capabilities.

* [ ] **Certificate Rotation:** Implement the `CertificateSigner` interface integration for automated, zero-downtime rotation.
* [ ] **Hybrid Caching:** Integrate `memguard` for secure in-memory caching of Fast Path credentials (utilizing Context Hash).
* [ ] **Mode B Support:** Implement `X-Forwarded-Client-Cert` trust logic for chained proxies.
* [ ] **Helm Charts:** Official K8s packaging.
