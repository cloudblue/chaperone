# Deployment Guide

How to deploy Chaperone in your infrastructure using Docker. This guide
covers building Docker images, running containers, production hardening,
and Kubernetes integration.

If you've followed the [Plugin Development Guide](plugin-development.md),
you already have a working proxy binary. This guide shows how to package
it into a production-ready container.

> **First time?** Complete the [Getting Started](../getting-started.md)
> tutorial and the [Plugin Development Guide](plugin-development.md)
> before deploying.

## Prerequisites

| Requirement | Version | Purpose |
|-------------|---------|---------|
| **Go** | 1.25+ | Building custom binaries |
| **Docker** | 20.10+ | Containerized deployment |
| **A working plugin project** | — | From the [Plugin Development Guide](plugin-development.md) |

For production mTLS deployments, you'll also need:

| Requirement | Purpose |
|-------------|---------|
| **Server certificate + key** (ECDSA P-256) | mTLS termination |
| **CA certificate** | Validating client certificates |
| **Outbound HTTPS** | Reaching vendor APIs |

## How to Build the Docker Image

### Default binary (reference plugin)

If you want to run Chaperone with the built-in reference plugin (reads
credentials from a JSON file), the repository already includes a
production-ready Dockerfile:

```bash
cd chaperone
docker build -t chaperone:latest .
```

This builds the `cmd/chaperone/main.go` entry point with the reference
plugin. Suitable for testing or simple deployments where credentials
come from a mounted JSON file.

### Custom plugin binary

If you built a custom plugin following the
[Plugin Development Guide](plugin-development.md), you need a Dockerfile
that compiles **your** code. This is the typical production scenario.

Create `Dockerfile` in your plugin project root (`my-proxy/`):

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build

# Copy your project and build.
# Go downloads Chaperone modules automatically from the Go module proxy.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o proxy .

# --- Runtime stage ---
# Distroless: no shell, no package manager, minimal attack surface.
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/proxy /app/proxy
USER nonroot:nonroot
EXPOSE 8443 9090
ENTRYPOINT ["/app/proxy"]
```

Build and run:

```bash
cd my-proxy
docker build -t my-proxy:latest .
```

That's it — Go resolves `github.com/cloudblue/chaperone` and
`github.com/cloudblue/chaperone/sdk` from the module proxy during the
build. No local Chaperone source needed.

#### Pre-publication: local Chaperone source

If Chaperone modules are **not yet published** on a Go package registry
(e.g., during early adoption), your `go.mod` uses `replace` directives
that point to a local copy of the Chaperone source
(see [Plugin Development Guide — Step 2](plugin-development.md#step-2-initialize-the-go-module)).
The Dockerfile needs access to the Chaperone source alongside your
project. Use Docker BuildKit's `--build-context` flag to pull it in
without changing your working directory:

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build

# Pull Chaperone source from the named build context (--build-context).
# Needed because go.mod has: replace ... => ../chaperone
COPY --from=chaperone / ./chaperone/

# Copy your plugin project.
COPY . ./my-proxy/
WORKDIR /build/my-proxy

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o proxy .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/my-proxy/proxy /app/proxy
USER nonroot:nonroot
EXPOSE 8443 9090
ENTRYPOINT ["/app/proxy"]
```

Build from your project directory, passing the Chaperone source as a
named context:

```bash
cd my-proxy
docker build -t my-proxy:latest \
  --build-context chaperone=../chaperone \
  .
```

> **Requires Docker BuildKit** (default since Docker 23+). If you're on
> an older version, enable it with `DOCKER_BUILDKIT=1` or build from the
> parent directory instead:
> `cd ~/projects && docker build -t my-proxy:latest -f my-proxy/Dockerfile .`

> **Once Chaperone is published**, remove the `replace` directives from
> your `go.mod`, switch to the simpler Dockerfile above, and build
> normally with `docker build -t my-proxy:latest .`.

## How to Configure for Deployment

### Configuration file

Chaperone reads its configuration from a YAML file. The repository
includes an example at `configs/config.example.yaml`. Copy it and
customize:

```bash
cp configs/config.example.yaml config.yaml
```

At minimum, configure these three settings:

1. **TLS certificates** — paths to your server cert, key, and CA bundle
2. **Allow-list** — which vendor API hosts and paths are permitted
3. **Log level** — `info` for production, `debug` for troubleshooting

```yaml
server:
  addr: ":8443"
  admin_addr: ":9090"
  tls:
    enabled: true
    cert_file: "/app/certs/server.crt"
    key_file: "/app/certs/server.key"
    ca_file: "/app/certs/ca.crt"

upstream:
  allow_list:
    "httpbin.org":
      - "/**"
    "api.vendor.com":
      - "/v1/**"
      - "/v2/**"

observability:
  log_level: "info"
```

The allow-list controls which hosts and paths the proxy is allowed to
reach. Requests to unlisted destinations get a `403 Forbidden`. See the
[Configuration Reference](../reference/configuration.md) for all
available options.

### Certificates

**For development/testing**, generate self-signed certificates:

```bash
cd chaperone
make gencerts
```

**For production**, use the enrollment flow to generate a CSR and have
it signed by your CA. See
[Certificate Management](certificate-management.md) for the complete
workflow.

## How to Run with Docker

### Start the container

Run the container with your config and certificates mounted as volumes:

```bash
docker run -d --name chaperone-proxy \
  -p 8443:8443 \
  -p 9090:9090 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  -v $(pwd)/certs:/app/certs:ro \
  -e CHAPERONE_CONFIG=/app/config.yaml \
  --read-only \
  my-proxy:latest
```

What each flag does:

| Flag | Purpose |
|------|---------|
| `-d` | Run in the background (detached) |
| `-p 8443:8443` | Expose the proxy traffic port |
| `-p 9090:9090` | Expose the admin port (health, metrics) |
| `-v ...config.yaml:ro` | Mount your config as read-only |
| `-v ...certs:ro` | Mount certificates as read-only |
| `-e CHAPERONE_CONFIG=...` | Tell the proxy where to find the config inside the container |
| `--read-only` | Make the container filesystem read-only (security hardening) |

> **For local testing without TLS**, use the same `config.yaml` from the
> plugin tutorial (with `tls.enabled: false`) and skip the certs volume.

### Verify the container is running

Check that the proxy started successfully:

```bash
# 1. Health check (admin port — no mTLS required)
curl -s http://localhost:9090/_ops/health
# {"status": "alive"}

# 2. Version info
curl -s http://localhost:9090/_ops/version
# {"version": "0.1.0", ...}
```

If TLS is disabled (local testing), test the proxy with the same curl
from the [Plugin Development Guide](plugin-development.md#step-7-run-and-verify):

```bash
curl http://localhost:8443/proxy \
  -H "X-Connect-Target-URL: https://httpbin.org/headers" \
  -H "X-Connect-Vendor-ID: test-vendor"
```

If TLS is enabled (production), include the client certificate:

```bash
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     -H "X-Connect-Target-URL: https://httpbin.org/headers" \
     -H "X-Connect-Vendor-ID: test-vendor" \
     https://localhost:8443/proxy
```

See the [HTTP API Reference](../reference/http-api.md) for full endpoint
documentation.

### Health checking in Docker

The distroless runtime image has **no shell**, so Docker can't run
health checks from inside the container. Use `curl` from the host
against the admin port instead:

```bash
curl -sf http://localhost:9090/_ops/health
```

For automated health checking in production, use Kubernetes HTTP probes
(below) or external monitoring tools.

## How to Deploy on Kubernetes

### Health probes

Chaperone exposes `/_ops/health` on both the traffic port (8443, requires
mTLS) and the admin port (9090, no mTLS). Configure Kubernetes probes
against the **admin port** so the kubelet doesn't need a client certificate:

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

The liveness probe restarts the pod if the proxy stops responding.
The readiness probe removes the pod from the Service endpoints until it's
ready to accept traffic — useful during startup and rolling updates.

### Production hardening

The Dockerfile is hardened by default:

- **Non-root execution** — runs as `nonroot:nonroot` (UID 65532)
- **Distroless base** — `gcr.io/distroless/static` has no shell, no
  package manager, minimal attack surface
- **Read-only mounts** — certificates and config mounted with `:ro`
- **Read-only filesystem** — `--read-only` flag prevents any writes
- **Small image** — ~50 MB total (static Go binary + distroless base)

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

If you prefer running the binary directly (without Docker), build it
from the Chaperone repository:

```bash
# Clone the repository
git clone https://github.com/cloudblue/chaperone.git
cd chaperone

# Install development tools (golangci-lint)
make tools

# Build and run with the reference plugin
make run
```

This builds `cmd/chaperone/main.go` and starts the proxy with the
default configuration. For custom plugins, see the
[Plugin Development Guide](plugin-development.md).

## Troubleshooting

If you encounter issues during deployment, see the
[Troubleshooting Guide](troubleshooting.md).

## Next Steps

- [Certificate Management](certificate-management.md) — Production certificate enrollment
- [Configuration Reference](../reference/configuration.md) — Full configuration specification
- [Plugin Development Guide](plugin-development.md) — Build your own credential plugin
- [Troubleshooting](troubleshooting.md) — Common issues and solutions
