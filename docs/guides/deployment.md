# Deployment Guide

How to deploy Chaperone in your infrastructure using Docker. This guide
covers building images, configuring containers, production hardening,
and Kubernetes integration.

> **First time?** Start with the [Getting Started](../getting-started.md) tutorial.

## Prerequisites

| Requirement | Version | Purpose |
|-------------|---------|---------|
| **Go** | 1.25+ | Building custom binaries |
| **Docker** | 20.10+ | Containerized deployment |
| **Certificates** | ECDSA P-256 | mTLS server and CA certificates |
| **Network access** | — | Outbound HTTPS to vendor APIs |

## How to Build the Docker Image

```bash
docker build -t chaperone:latest .
```

If you have a custom plugin binary (see [Plugin Development](plugin-development.md)):

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

## How to Configure for Deployment

### Configuration File

Copy the example configuration and customize it:

```bash
cp configs/config.example.yaml config.yaml
```

At minimum, configure:

1. **TLS certificates** — paths to your server cert, key, and CA
2. **Allow-list** — which vendor API hosts and paths are permitted
3. **Log level** — `info` for production, `debug` for troubleshooting

```yaml
upstream:
  allow_list:
    "api.vendor.com":
      - "/v1/**"
      - "/v2/**"
```

See the [Configuration Reference](../reference/configuration.md) for all
available options.

### Certificates

Generate development certificates for testing:

```bash
make gencerts
```

For production, use CA enrollment. See [Certificate Management](certificate-management.md)
for the complete certificate workflow.

## How to Run with Docker

### Basic Container Run

```bash
docker run -d --name chaperone-proxy \
  -p 8443:8443 \
  -p 9090:9090 \
  -v $(pwd)/certs:/app/certs:ro \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  --read-only \
  chaperone:latest
```

Port mapping:
- **8443** — Proxy traffic port (mTLS)
- **9090** — Admin port (health, version, metrics, pprof)

Certificates and configuration are mounted as read-only (`:ro`).
Combined with `--read-only`, the container has no writable paths.

### Health Checking

The distroless image has **no shell** — there is no `curl`, `wget`, or `sh`
inside the container. Use host-side or orchestrator probes instead.

**From the host:**

```bash
curl -s http://localhost:9090/_ops/health
# {"status": "alive"}
```

**Docker health check (compose or CLI):**

```yaml
# docker-compose.yml
services:
  chaperone:
    healthcheck:
      test: ["CMD", "/app/chaperone", "-version"]
      interval: 30s
      timeout: 5s
      retries: 3
```

> **Note:** The `-version` check only verifies the binary runs, not that the
> server is accepting traffic. Since distroless has no shell or `curl`, this
> is the best option within Compose. For production, use Kubernetes HTTP
> probes (below) or external monitoring against the admin port.

### Verification Steps

After starting the container, verify all endpoints:

```bash
# 1. Health (admin port, no mTLS required)
curl -s http://localhost:9090/_ops/health
# {"status": "alive"}

# 2. Version (admin port, no mTLS required)
curl -s http://localhost:9090/_ops/version
# {"version": "..."}

# 3. Test proxy request (requires mTLS client cert)
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     -H "X-Connect-Target-URL: https://api.vendor.com/v1/test" \
     -H "X-Connect-Vendor-ID: test-vendor" \
     https://localhost:8443/proxy
```

See the [HTTP API Reference](../reference/http-api.md) for full endpoint
documentation.

## How to Deploy on Kubernetes

### Health Probes

Both `/_ops/health` and `/_ops/version` are available on the traffic port
(8443, requires mTLS) and the admin port (9090, no mTLS). Configure probes
on the admin port:

```yaml
livenessProbe:
  httpGet:
    path: /_ops/health
    port: 9090
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /_ops/health
    port: 9090
  initialDelaySeconds: 3
  periodSeconds: 5
```

### Production Hardening

The Dockerfile is hardened by default:

- Runs as `nonroot:nonroot` (UID 65532)
- Uses `gcr.io/distroless/static` — no shell, minimal attack surface
- Mount certs and config as read-only (`:ro`)
- Use `--read-only` for a read-only root filesystem
- Final image is ~50 MB (static Go binary + distroless base)

## How to Scale with a Load Balancer

Chaperone supports two deployment modes: **Mode A** (standalone mTLS
termination) and **Mode B** (HTTP behind a load balancer that terminates
mTLS). See the [Design Specification](../explanation/DESIGN-SPECIFICATION.md)
for background on these modes.

Each instance is stateless. The in-memory credential cache is a
performance optimization, not critical state — a cache miss triggers
plugin execution with no data loss. You can run multiple instances in an
active/active configuration and scale by adding instances behind a load
balancer.

### Mode A: mTLS pass-through (L4 load balancer)

When Chaperone terminates mTLS directly (Mode A), the load balancer
must pass through TCP connections without terminating TLS. Use a
**Layer 4** load balancer:

- **AWS:** Network Load Balancer (NLB) with TCP listeners
- **Kubernetes:** `Service` of type `LoadBalancer` or `NodePort` (not `Ingress` — standard Ingress terminates TLS)
- **HAProxy:** `mode tcp` with `option tcplog`

```
Client (mTLS) ──TCP──▸ L4 Load Balancer ──TCP──▸ Chaperone instance 1
                                          ──TCP──▸ Chaperone instance 2
                                          ──TCP──▸ Chaperone instance N
```

Configure the load balancer health check against the admin port
(`/_ops/health` on port 9090), not the traffic port — the admin port
does not require mTLS.

### Mode B: HTTP behind a reverse proxy (L7)

Chaperone also runs in plain HTTP mode behind a reverse proxy or
load balancer that terminates mTLS on behalf of the proxy. The core
request flow (context extraction, credential injection, routing) is
transport-agnostic.

```yaml
server:
  addr: ":8080"
  tls:
    enabled: false
```

```
Client (mTLS) ──TLS──▸ Nginx / ALB ──HTTP──▸ Chaperone :8080
```

In this mode, restrict network access so only the upstream reverse
proxy can reach Chaperone — do not expose the HTTP port to the public
internet.

> **Note:** Production-grade identity forwarding for Mode B
> (`X-Forwarded-Client-Cert` trust validation) is planned for a future
> release. Mode B works when the network
> boundary provides sufficient isolation.

### Rolling updates

Chaperone supports zero-downtime rolling updates. On `SIGTERM`, an
instance stops accepting new connections and drains in-flight requests
within `shutdown_timeout` (default 30s) before exiting. The update
sequence:

1. Start a new instance. It passes the health probe and joins the pool.
2. Send `SIGTERM` to the old instance. The load balancer detects it as
   unhealthy via `/_ops/health` and stops routing new traffic to it.
3. The old instance finishes in-flight requests and exits.
4. Repeat per instance.

Since instances share no state, the orchestrator can cycle them
independently.

## How to Build from Source

```bash
# Clone the repository
git clone https://github.com/cloudblue/chaperone.git
cd chaperone

# Install development tools (golangci-lint)
make tools

# Build and run
make run
```

## Troubleshooting

If you encounter issues during deployment, see the
[Troubleshooting Guide](troubleshooting.md).

## Next Steps

- [Certificate Management](certificate-management.md) — Production certificate enrollment
- [Configuration Reference](../reference/configuration.md) — Full configuration specification
- [Plugin Development Guide](plugin-development.md) — Build your own credential plugin
- [Troubleshooting](troubleshooting.md) — Common issues and solutions
