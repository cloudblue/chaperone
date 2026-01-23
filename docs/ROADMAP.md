# Project Roadmap & Delivery Stages

We define three key milestones to validate the architecture and reach production readiness.

## Phase 1: Proof of Concept (PoC)

**Goal:** Validate the "Static Recompilation" architecture, mTLS Handshake, and Context Logic using a **Docker-only environment**.

* [ ] **Core Skeleton:** Implement a basic Go HTTP Server using `httputil.ReverseProxy`.
* [ ] **Context Parsing:** Implement the `TransactionContext` extractor for `X-Connect-*` headers.
* [ ] **Context Hashing:** Implement the deterministic hashing logic (canonicalization) of the Context to validate the caching strategy inputs (even if storage is deferred).
* [ ] **Plugin Mechanism:** Create a dummy `proxy-sdk` and a hardcoded plugin to verify the compiler can build a single binary from two sources.
* [ ] **mTLS Verification:** Create a Go test using `httptest` to simulate a Connect handshake with client certificates.
* [ ] **Docker Validation:** Verify the PoC compiles and runs successfully inside the standard `Dockerfile` container.

## Phase 2: Minimum Viable Product (MVP)

**Goal:** A secure, distributable version that Early Adopter Distributors can deploy in "Mode A".

* [ ] **Module Separation:** Formally publish `github.com/connect/proxy-sdk` (v0.1) and `proxy-core`.
* [ ] **Configuration:** Implement `config.yaml` loading with Environment Variable overrides.
* [ ] **Router (Allow-List):** Implement the logic that maps `Service-ID` to specific upstream URLs (Destination Lookup), enforcing the "Default Deny" rule.
* [ ] **Error Normalization:** Implement the middleware to intercept upstream `500/400` errors, hiding stack traces and returning sanitized JSON responses.
* [ ] **Security Layer:** Implement the "Redactor" middleware (for logs) and "Reflector" protection (stripping Auth headers from responses).
* [ ] **Reference Plugin:** Build the `ReferenceProvider` that reads JSON file credentials (file-based auth).
* [ ] **Observability (Logs):** Implement structured JSON logging to `STDOUT` (Trace ID, Status, Latency).
* [ ] **Resilience (Static):** Apply hardcoded safe defaults for Timeouts (e.g., 30s) to prevent hanging connections.
* [ ] **Deployment Assets:** Create the `Dockerfile` (Primary) and static reference files for K8s (`deployment.yaml`) and Systemd (`.service`) for the README.

## Phase 3: General Availability (V1.0 Production)

**Goal:** Operational excellence, performance hardening, and full observability.

* [ ] **Certificate Rotation:** Implement the `CertificateSigner` interface integration for automated, zero-downtime rotation.
* [ ] **Telemetry (Metrics):** Expose the `/metrics` endpoint with Prometheus counters (e.g., `requests_total`).
* [ ] **Resilience (Configurable):** Move Timeout settings (Read/Write/Idle) from hardcoded defaults to `config.yaml`.
* [ ] **Hybrid Caching:** Integrate `memguard` for secure in-memory caching of Fast Path credentials (utilizing the Context Hash verified in PoC).
* [ ] **Performance Attribution:** Implement the `Server-Timing` header logic to visualize "Plugin vs. Upstream" latency.
* [ ] **Profiling:** Add the optional `/debug/pprof` endpoint (protected by config).
* [ ] **Documentation:** Publish the "Distributor Integration Guide" and "Plugin Developer Handbook".

## Phase 4: Future Scope (Post-V1.0)

* [ ] **Mode B Support:** Implement `X-Forwarded-Client-Cert` trust logic for chained proxies.
* [ ] **Helm Charts:** Official K8s packaging.
* [ ] **OpenTelemetry:** Native OTLP exporters for Push-based telemetry.
