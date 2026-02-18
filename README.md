<div align="center">
  <img width="1920" height="829" alt="chaperone_banner" src="https://github.com/user-attachments/assets/a4fbfb21-5776-4a03-a5b2-91586fa0b0c4" />
  <p align="center">
    <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go"></a>
    <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg">
    <img src="https://github.com/cloudblue/chaperone/actions/workflows/ci.yml/badge.svg?branch=master">
    <img src="https://github.com/cloudblue/chaperone/actions/workflows/security.yml/badge.svg?branch=master">
  </div>
  <br/>
</div>



# Chaperone

**Chaperone** is a high-performance, sidecar-style egress proxy designed to securely inject credentials into outgoing API requests. It runs within your infrastructure to manage sensitive authentication data (tokens, API keys, OAuth2 credentials) without exposing them to upstream platforms.

## Features

- 🔐 **Secure Credential Injection** - Credentials never leave your infrastructure
- 🔒 **Mutual TLS (mTLS)** - Full support for client certificate authentication
- ⚡ **High Performance** - Built in Go with efficient connection handling
- 🔌 **Plugin Architecture** - Extensible via static recompilation (no runtime dependencies)
- 📊 **Observable** - Structured logging and metrics support
- 🐳 **Cloud Native** - Designed for containerized deployments

## Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Tutorial: build, run, and send your first request |
| [Deployment Guide](docs/guides/deployment.md) | Docker deployment, Kubernetes probes, production hardening |
| [Configuration Reference](docs/reference/configuration.md) | All config options, env overrides, timeout tuning |
| [Plugin Development Guide](docs/guides/plugin-development.md) | Build your own credential plugin |
| [HTTP API Reference](docs/reference/http-api.md) | All endpoints — health, version, proxy, metrics |
| [SDK Reference](docs/reference/sdk.md) | Plugin interfaces, types, public API |
| [Troubleshooting](docs/guides/troubleshooting.md) | Common issues and solutions |

## Project Status

🚧 **Work in Progress** - Phase 1 (PoC) is complete. Currently in **Phase 2 (MVP)**.

## Requirements

- Go 1.25 or higher
- Docker (for containerized deployment)
- jq (for `make docker-test` validation suite)

## Quick Start

### Docker (Recommended)

```bash
# Build the image
docker build -t chaperone:latest .

# Run with the tutorial config (allows httpbin.org as target)
docker run --rm -p 8443:8443 -p 9090:9090 \
  -v $(pwd)/configs/getting-started.yaml:/app/config.yaml:ro \
  chaperone:latest

# In a new terminal — health check
curl -s http://localhost:9090/_ops/health
# {"status": "alive"}

# Proxy a request through Chaperone to httpbin.org
curl -s http://localhost:8443/proxy \
  -H "X-Connect-Target-URL: https://httpbin.org/headers" \
  -H "X-Connect-Vendor-ID: test-vendor"
```

> **Note:** Chaperone requires an allow-list of permitted target hosts
> (security-by-default). The tutorial config permits `httpbin.org` only.
> See the [Getting Started](docs/getting-started.md) tutorial for the full walkthrough.

### With mTLS (Production)

```bash
# Generate test certificates first
make gencerts

# Run with certificates mounted
docker run -p 8443:8443 \
  -v $(pwd)/certs:/app/certs:ro \
  chaperone:latest -tls=true

# Test with client certificate
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     https://localhost:8443/_ops/health
```

> **Note (macOS):** The built-in curl uses LibreSSL which may fail with ECDSA client certificates.
> If you see `bad decrypt` errors, use the Docker-based curl instead:
> ```bash
> docker run --rm --network host -v $(pwd)/certs:/certs:ro curlimages/curl \
>     --cacert /certs/ca.crt --cert /certs/client.crt --key /certs/client.key \
>     https://localhost:8443/_ops/health
> ```

### From Source

```bash
# Clone the repository
git clone https://github.com/cloudblue/chaperone.git
cd chaperone

# Install development tools (golangci-lint)
make tools

# Build and run
make run
```

## Project Structure

This is a **multi-module monorepo** with independent versioning for Core and SDK:

```
chaperone/
├── chaperone.go            # Public API: Run(), Option types
├── cmd/chaperone/          # Default CLI entry point (wraps chaperone.Run)
├── sdk/                    # Plugin SDK (separate Go module)
│   ├── go.mod              # Versioned independently (sdk/v1.x.x)
│   ├── plugin.go           # Plugin interfaces
│   └── compliance/         # Test kit for Distributors
├── internal/               # Private application code
│   ├── config/             # Configuration handling
│   ├── proxy/              # Core proxy logic
│   ├── cache/              # Credential cache (memguard)
│   └── observability/      # Logging, metrics, tracing
├── plugins/reference/      # Default file-based plugin
└── configs/                # Example configuration files
```

### Building Custom Binaries (Distributor Workflow)

Distributors create their own binary by importing the SDK and the core:

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/cloudblue/chaperone"
    "github.com/cloudblue/chaperone/sdk"
)

// MyPlugin implements sdk.Plugin with your credential logic.
type MyPlugin struct{}

func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    // Your credential injection logic here
    return &sdk.Credential{Headers: map[string]string{"Authorization": "Bearer my-token"}}, nil
}

func (p *MyPlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
    return nil, fmt.Errorf("certificate signing not implemented")
}

func (p *MyPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
    return nil, nil // Default behavior: Core applies error normalization
}

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := chaperone.Run(ctx, &MyPlugin{},
        chaperone.WithConfigPath("/etc/chaperone.yaml"),
        chaperone.WithVersion("1.0.0"),
    ); err != nil {
        os.Exit(1)
    }
}
```

Available options:
- `WithConfigPath(path)` — Path to YAML config file
- `WithVersion(version)` — Version string for `/_ops/version` endpoint
- `WithBuildInfo(commit, date)` — Git metadata for startup logs
- `WithLogOutput(writer)` — Custom log output (default: `os.Stdout`)

For the complete guide including "Own Repo" and "Fork/Extend" workflows,
testing, and common credential patterns, see the
[Plugin Development Guide](docs/guides/plugin-development.md).

## Configuration

Chaperone is configured via a YAML file (`config.yaml`) with environment variable overrides following the 12-Factor App methodology.

### Configuration File

By default, Chaperone looks for configuration in this order:
1. Path specified via `-config` flag
2. Path in `CHAPERONE_CONFIG` environment variable
3. `./config.yaml` in the current directory

See [configs/config.example.yaml](configs/config.example.yaml) for a complete annotated example.

### Quick Start Configuration

```yaml
server:
  addr: ":8443"
  tls:
    cert_file: "certs/server.crt"
    key_file: "certs/server.key"
    ca_file: "certs/ca.crt"

upstream:
  allow_list:
    "api.vendor.com":
      - "/v1/**"
      - "/v2/**"

observability:
  log_level: "info"
```

### Configuration Reference

For the complete configuration specification with all options, defaults,
environment variable overrides, and timeout tuning guidance, see the
[Configuration Reference](docs/reference/configuration.md).

**Key configuration areas:**

- **Server** — Traffic port, admin port, shutdown timeout, TLS certificates
- **Upstream** — Header prefix, allow-list (default-deny), timeouts
- **Observability** — Log level, profiling, sensitive header redaction

Environment variables override YAML values using the pattern
`CHAPERONE_<SECTION>_<KEY>` (uppercase, underscore separator).

```bash
export CHAPERONE_SERVER_ADDR=":9443"
export CHAPERONE_OBSERVABILITY_LOG_LEVEL="debug"
export CHAPERONE_UPSTREAM_TIMEOUTS_CONNECT="10s"
```

## Certificate Management

Chaperone provides two tools for certificate management depending on your use case:

### Development (Self-Signed Certificates)

Use `make gencerts` for local development and testing:

```bash
# Generate self-signed test certificates (ECDSA P-256)
make gencerts

# With custom domains/IPs for the server certificate:
make gencerts DOMAINS="myserver.local,192.168.1.100"
```

This creates a `certs/` directory with:
- `ca.crt` - Test CA certificate (for client verification)
- `server.crt` - Server certificate (self-signed by test CA)
- `server.key` - Server private key
- `client.crt` - Client certificate (for curl testing)
- `client.key` - Client private key

**Test with curl:**
```bash
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     https://localhost:8443/_ops/health
```

### Production (CA Enrollment)

Use `chaperone enroll` for production deployments:

```bash
# Generate key pair and CSR for your domain
./chaperone enroll --domains proxy.example.com

# Multiple domains/IPs
./chaperone enroll --domains proxy.example.com,10.0.0.1 --cn my-proxy

# Custom output directory
./chaperone enroll --domains proxy.example.com --out /etc/chaperone/certs

# Overwrite existing key/CSR files
./chaperone enroll --domains proxy.example.com --force
```

This generates:
- `server.key` - Private key (keep secure, never share)
- `server.csr` - Certificate Signing Request (submit to CA)

**Enrollment workflow:**
1. Run `chaperone enroll --domains your.domain.com`
2. Submit `server.csr` to your CA (Connect Portal, Vault, etc.)
3. Place the signed `server.crt` in the certs directory
4. Start Chaperone: `./chaperone`

## Development

```bash
# Install development tools (golangci-lint)
make tools

# Build for development (allows HTTP targets, debug symbols)
make build-dev

# Build for production (HTTPS targets only, stripped)
make build

# Build and run
make run

# Generate test certificates for mTLS development
make gencerts
# With custom domains/IPs:
make gencerts DOMAINS="myserver.local,192.168.1.100"

# Run tests
make test

# Run tests with race detector
make test-race

# Run tests with coverage report
make test-cover

# Run short tests only
make test-short

# Run linters
make lint

# Run linters and auto-fix issues
make lint-fix

# Format code
make fmt

# Run go vet
make vet

# Tidy and verify go.mod
make tidy

# Remove build artifacts
make clean

# Show all available commands
make help
```

### Docker Development

```bash
# Build Docker image
make docker-build

# Run Docker container (HTTP mode for testing)
make docker-run

# Run the Docker Validation Suite (comprehensive end-to-end testing)
make docker-test

# Check Docker image size
make docker-size

# Clean up Docker images (production, test, and echoserver)
make docker-clean
```

#### Docker Validation Suite

`make docker-test` runs a comprehensive validation suite that builds both a test image (HTTP targets allowed) and a production image (HTTPS-only), spins up an isolated Docker network with an echo server, and verifies the full proxy lifecycle:

| # | Category | Test | Validates |
|---|----------|------|-----------|
| 1–5 | **Setup & Health** | Network, echo server, proxy startup, health & version endpoints | Container boots correctly, admin endpoints respond |
| 6–9 | **Proxy Round-Trip** | Bearer credential injection, path forwarding, HTTP method passthrough | Core proxy logic works end-to-end |
| 10–12 | **Security & Compliance** | Non-root user, distroless base (no shell), image size < 50MB | Production hardening |
| 13 | **Telemetry** | Prometheus `/metrics` endpoint format, `chaperone_requests_total` counter, `chaperone_request_duration_seconds` histogram | Metrics wiring and observability |
| 14–15 | **Request Validation** | Missing target URL → 400, blocked host → 403 | Input validation and allow-list enforcement |
| 16 | **Secure Defaults** | Production image rejects HTTP targets (400) | Build-time security flag (`ALLOW_INSECURE_TARGETS`) |
| 17–18 | **Operational** | Graceful shutdown (SIGTERM → exit 0), malformed config rejection | Runtime resilience |

All containers and networks are cleaned up automatically on exit.

## Architecture

Chaperone follows a plugin-based architecture where credential providers are compiled directly into the binary (similar to the Caddy model). This approach provides:

- Zero network serialization overhead
- Full access to Go's ecosystem
- Single static binary deployment
- No runtime dependency conflicts

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting pull requests.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
