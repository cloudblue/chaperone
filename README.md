<div align="center">
  <img width="1920" height="829" alt="chaperone_banner" src="https://github.com/user-attachments/assets/a4fbfb21-5776-4a03-a5b2-91586fa0b0c4" />
  <p align="center">
    <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go"></a>
    <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg">
    <img src="https://github.com/cloudblue/chaperone/actions/workflows/ci.yml/badge.svg?branch=master">
  </div>
  <br/>
</div>

# Chaperone

**Chaperone** is a sidecar-style egress proxy that injects credentials into outgoing API requests. It runs within your infrastructure so that tokens, API keys, and OAuth2 credentials never reach the upstream platform.

- **Credential isolation** — Secrets stay in your infrastructure. The proxy adds them to outgoing requests and strips them from responses.
- **Mutual TLS** — Client certificate authentication between the platform and the proxy, with TLS 1.3 minimum.
- **Plugin architecture** — Credential providers compile directly into the binary (static recompilation, similar to Caddy). No runtime plugin loading, no serialization overhead.
- **Structured observability** — JSON logging via `log/slog`, Prometheus metrics, and health/version endpoints.
- **Single static binary** — Runs on a distroless container image under 50 MB.

## Quick start

```bash
# Build the Docker image
docker build -t chaperone:latest .

# Run with the tutorial config (allows httpbin.org as target)
docker run --rm -p 8443:8443 -p 9090:9090 \
  -v $(pwd)/configs/getting-started.yaml:/app/config.yaml:ro \
  chaperone:latest

# In another terminal — health check
curl -s http://localhost:9090/_ops/health
# {"status": "alive"}

# Proxy a request to httpbin.org
curl -s http://localhost:8443/proxy \
  -H "X-Connect-Target-URL: https://httpbin.org/headers" \
  -H "X-Connect-Vendor-ID: test-vendor"
```

Chaperone requires an allow-list of permitted target hosts (default-deny). The tutorial config permits `httpbin.org` only. See [Getting Started](docs/getting-started.md) for the full walkthrough including credential injection.

To build from source instead:

```bash
make tools && make build-dev
./bin/chaperone -config configs/getting-started.yaml
```

## Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Tutorial: build, run, and send your first proxied request |
| [Plugin Development](docs/guides/plugin-development.md) | Build a custom credential plugin |
| [Deployment](docs/guides/deployment.md) | Docker deployment, Kubernetes probes, production hardening |
| [Certificate Management](docs/guides/certificate-management.md) | mTLS setup, `chaperone enroll`, CA workflows |
| [Configuration Reference](docs/reference/configuration.md) | All config options, env overrides, timeout tuning |
| [HTTP API Reference](docs/reference/http-api.md) | Health, version, proxy, and metrics endpoints |
| [SDK Reference](docs/reference/sdk.md) | Plugin interfaces, types, and public API |
| [Contrib Plugins Reference](docs/reference/contrib-plugins.md) | Reusable auth building blocks, request multiplexer |
| [Troubleshooting](docs/guides/troubleshooting.md) | Common issues and solutions |
| [Design Specification](docs/explanation/DESIGN-SPECIFICATION.md) | Architecture rationale and ADRs |

## Project structure

Chaperone is a multi-module monorepo. The Core and SDK are versioned independently so that plugin authors can depend on a stable SDK without pulling in proxy internals.

```
chaperone/
├── chaperone.go            # Public API: Run(), Option types
├── cmd/chaperone/          # CLI entry point (wraps chaperone.Run)
├── sdk/                    # Plugin SDK (separate Go module, sdk/v1.x.x)
│   ├── plugin.go           # Plugin, CredentialProvider, CertificateSigner interfaces
│   └── compliance/         # Contract test kit for plugin authors
├── internal/               # Private implementation
│   ├── proxy/              # Reverse proxy, TLS termination, middleware
│   ├── config/             # YAML + env var configuration
│   ├── router/             # Request routing and validation
│   ├── telemetry/          # Prometheus metrics, admin server
│   └── ...                 # cache, context, observability, security
├── plugins/reference/      # Default file-based credential provider
├── plugins/contrib/        # Reusable auth building blocks (OAuth2, Microsoft SAM, mux)
└── configs/                # Example configuration files
```

## Development

```bash
make tools        # Install dev tools (golangci-lint, goimports)
make build-dev    # Dev build (HTTP targets allowed, debug symbols)
make build        # Production build (HTTPS-only, stripped)
make test         # Run all tests
make test-race    # Tests with race detector
make lint         # Run linters (Core + SDK modules)
make fmt          # Format code
make gosec        # Run gosec security scanner
make govulncheck  # Run govulncheck vulnerability scanner
make ci           # Run all CI checks locally
make gencerts     # Generate test certificates for mTLS
make docker-test  # End-to-end Docker validation suite (18 tests)
make help         # Show all available targets
```

## License

This project is licensed under the Apache License 2.0 — see the [LICENSE](LICENSE) file for details.
