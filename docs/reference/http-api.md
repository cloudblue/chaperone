# HTTP API Reference

Complete reference for all HTTP endpoints exposed by Chaperone. The proxy
exposes two server ports: a **traffic port** (mTLS-protected) and an
**admin port** (plain HTTP, bound to localhost by default).

## Ports

| Port | Default | Auth | Purpose |
|------|---------|------|---------|
| Traffic | `:443` | mTLS (client cert required) | Proxied API requests, health, version |
| Admin | `127.0.0.1:9090` | None (bind to localhost) | Health, version, metrics, profiling |

Both ports are configurable via `server.addr` and `server.admin_addr` in
the [Configuration Reference](configuration.md).

## Endpoint Summary

| Endpoint | Method | Traffic Port | Admin Port | Description |
|----------|--------|:---:|:---:|-------------|
| [`/proxy`](#-proxy) | Any | ✓ | — | Forward request with credential injection |
| [`/_ops/health`](#get-_opshealth) | GET | ✓ | ✓ | Health check |
| [`/_ops/version`](#get-_opsversion) | GET | ✓ | ✓ | Version info |
| [`/_ops/renew/prepare`](#post-_opsrenewprepare) | POST | ✓ | — | Initiate certificate renewal (Connect-managed only) |
| [`/_ops/renew/install`](#post-_opsrenewinstall) | POST | ✓ | — | Complete certificate renewal (Connect-managed only) |
| [`/metrics`](#get-metrics) | GET | — | ✓ | Prometheus metrics |
| [`/debug/pprof/*`](#profiling-endpoints-admin-port-only) | GET | — | ✓ | Profiling (dev builds only) |

---

## Proxy Endpoint (Traffic Port Only)

### `* /proxy`

The main proxy endpoint. Accepts **all HTTP methods** (GET, POST, PUT,
DELETE, PATCH, etc.) and forwards them to the target URL with credentials
injected.

**Request headers (required):**

| Header | Description |
|--------|-------------|
| `X-Connect-Target-URL` | Destination URL (must be in the allow-list) |

**Request headers (optional):**

| Header | Description |
|--------|-------------|
| `X-Connect-Vendor-ID` | Vendor account identifier |
| `X-Connect-Environment-ID` | Runtime environment (e.g., `production`, `test`) |
| `X-Connect-Product-ID` | Product SKU |
| `X-Connect-Marketplace-ID` | Marketplace identifier |
| `X-Connect-Subscription-ID` | Subscription identifier |
| `X-Connect-Context-Data` | Base64-encoded JSON with additional context |
| `Connect-Request-ID` | Correlation ID (auto-generated if absent) |

> **Note:** The `X-Connect-` prefix is configurable via `upstream.header_prefix`.
> The trace header name is configurable via `upstream.trace_header`.

**Example request:**

```bash
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     -H "X-Connect-Target-URL: https://api.vendor.com/v1/orders" \
     -H "X-Connect-Vendor-ID: vendor-123" \
     https://localhost:443/proxy
```

**Response:** The vendor's response is forwarded back, with:
- Sensitive headers (`Authorization`, `Cookie`, etc.) stripped from the response
- Plugin-injected credential headers stripped (Credential Reflection Protection)
- Upstream error bodies normalized (4xx/5xx bodies are replaced with generic messages to prevent information leakage)
- `Server-Timing` header added with proxy/upstream/plugin durations
- Trace ID echoed via the configured trace header

**Status codes:**

| Code | Meaning |
|------|---------|
| `200–299` | Success (from vendor) |
| `400` | Missing/invalid context headers, invalid target URL, or unsupported URL scheme |
| `403` | Target host or path not in the allow-list |
| `500` | Plugin error (credential fetch failed) |
| `502` | Upstream connection failure (vendor unreachable) |
| `504` | Plugin timeout (`upstream.timeouts.plugin` exceeded) |

**Response headers:**

| Header | Description |
|--------|-------------|
| `Server-Timing` | Timing breakdown: `plugin`, `upstream`, `overhead` durations (milliseconds) |

---

## Operational Endpoints

These endpoints are available on **both** the traffic port and admin port.

### `GET /_ops/health`

Returns the health status of the proxy.

**Request:**

```
GET /_ops/health HTTP/1.1
```

**Response:**

```
HTTP/1.1 200 OK
Content-Type: application/json

{"status": "alive"}
```

**Status codes:**

| Code | Meaning |
|------|---------|
| `200` | Server is healthy and accepting requests |

**Usage:** Kubernetes liveness/readiness probes, load balancer health checks.
Use the admin port (9090) for probes to avoid mTLS requirements.

### `GET /_ops/version`

Returns the proxy version.

**Request:**

```
GET /_ops/version HTTP/1.1
```

**Response:**

```
HTTP/1.1 200 OK
Content-Type: application/json

{"version":"1.0.0"}
```

The version string is set via `chaperone.WithVersion()` at startup.

**Status codes:**

| Code | Meaning |
|------|---------|
| `200` | Version returned successfully |

---

## Certificate Renewal Endpoints (Traffic Port Only)

These endpoints implement the Connect-driven certificate rotation protocol.
They are protected by the same mTLS listener as the proxy endpoint — a valid
Connect client certificate is required. Both endpoints return `501 Not
Implemented` when `server.tls.cert_management` is set to `external`.

### `POST /_ops/renew/prepare`

Initiates a certificate renewal. The proxy generates a fresh ECDSA P-256
key pair and CSR using the current certificate's Subject Alternative Names,
stores the pending state with a 10-minute TTL, and returns the CSR and a
pairing token (`renewal_id`).

**Request:** empty body.

**Response — success:**

```
HTTP/1.1 200 OK
Content-Type: application/json

{
  "csr": "-----BEGIN CERTIFICATE REQUEST-----\n...\n-----END CERTIFICATE REQUEST-----\n",
  "renewal_id": "a3f8e1c2d4b5..."
}
```

**Status codes:**

| Code | Meaning |
|------|---------|
| `200` | CSR and renewal_id returned; proceed to sign the CSR and call install |
| `501` | `cert_management: external` — renewal endpoints are disabled |

---

### `POST /_ops/renew/install`

Completes a certificate renewal. Connect submits the signed certificate
together with the `renewal_id` returned by prepare. The proxy validates the
certificate's public key against the pending private key, hot-swaps the TLS
listener, and atomically writes the new key and certificate to disk.

**Request body:**

```json
{
  "renewal_id": "a3f8e1c2d4b5...",
  "certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n"
}
```

**Response — success:** empty body, status `202 Accepted`.

**Status codes:**

| Code | Meaning |
|------|---------|
| `202` | Certificate installed and TLS listener hot-swapped |
| `409` | `renewal_id` mismatch or pending renewal has expired (TTL 10 min) |
| `422` | Certificate public key does not match the pending private key |
| `501` | `cert_management: external` — renewal endpoints are disabled |

---

## Metrics Endpoint (Admin Port Only)

### `GET /metrics`

Prometheus metrics endpoint. Returns metrics in the standard Prometheus
exposition format.

**Request:**

```
GET /metrics HTTP/1.1
```

**Metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `chaperone_requests_total` | Counter | `vendor_id`, `status_class`, `method` | Total requests processed |
| `chaperone_request_duration_seconds` | Histogram | `vendor_id` | End-to-end request latency |
| `chaperone_upstream_duration_seconds` | Histogram | `vendor_id` | Upstream (vendor API) latency |
| `chaperone_active_connections` | Gauge | — | Number of in-flight requests |
| `chaperone_panics_total` | Counter | — | Total recovered panics |

`status_class` is bucketed (`2xx`, `3xx`, `4xx`, `5xx`). `vendor_id` is
normalized to `[a-zA-Z0-9._-]` and truncated to 64 characters; invalid
values are replaced with `unknown`.

**Example:**

```bash
curl -s http://localhost:9090/metrics | head -20
```

---

## Profiling Endpoints (Admin Port Only)

Profiling requires **two gates**, both of which must be enabled:

1. **Build-time flag** — The binary must be compiled with profiling allowed:
   ```bash
   go build -ldflags "-X 'github.com/cloudblue/chaperone/internal/telemetry.allowProfiling=true'" ...
   ```
   Production builds omit this flag, making it impossible to enable profiling
   even if the config is changed.

2. **Runtime configuration** — Enable in the config file or env var:
   ```yaml
   observability:
     enable_profiling: true
   ```

When both gates are open, Go's standard `pprof` endpoints are registered:

| Path | Description |
|------|-------------|
| `GET /debug/pprof/` | Index of available profiles |
| `GET /debug/pprof/profile?seconds=30` | CPU profile |
| `GET /debug/pprof/heap` | Heap memory profile |
| `GET /debug/pprof/goroutine?debug=2` | Goroutine stack dump |
| `GET /debug/pprof/allocs` | Allocation profile |

**Example:**

```bash
# CPU profile (30 seconds)
go tool pprof http://localhost:9090/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:9090/debug/pprof/heap
```

> **Warning:** Profiling endpoints expose sensitive runtime information
> (heap dumps, goroutine stacks). Production builds should **never** set
> the `allowProfiling` build flag.
