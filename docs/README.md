# Chaperone Documentation

Welcome to the Chaperone documentation. These docs are organized using the
[Diátaxis](https://diataxis.fr/) framework into four categories: tutorials,
how-to guides, reference, and explanation.

## Tutorial

| Document | Description |
|----------|-------------|
| [Getting Started](getting-started.md) | Build, run, and send your first proxied request (~10 min) |

## How-to Guides

| Guide | Description |
|-------|-------------|
| [Deployment](guides/deployment.md) | Docker builds, container operations, Kubernetes probes, production hardening |
| [Certificate Management](guides/certificate-management.md) | Development certs, production CA enrollment, CSR generation |
| [Plugin Development](guides/plugin-development.md) | Build your own credential plugin — integration methods, testing, common patterns |
| [Troubleshooting](guides/troubleshooting.md) | Common errors, mTLS issues, allow-list debugging, Docker problems |

## Reference

| Document | Description |
|----------|-------------|
| [Configuration](reference/configuration.md) | All config options, env var overrides, allow-list syntax, timeout tuning |
| [HTTP API](reference/http-api.md) | All endpoints — health, version, proxy, metrics, profiling |
| [SDK](reference/sdk.md) | Plugin interfaces, types, helper methods, public API |

## Explanation

| Document | Description |
|----------|-------------|
| [Design Specification](explanation/DESIGN-SPECIFICATION.md) | Architecture, ADRs, security model, interface contracts |
| [Roadmap](ROADMAP.md) | Phased delivery plan (PoC → MVP → GA → Future) |

## Quick Start

1. **Try it:** Follow the [Getting Started](getting-started.md) tutorial
2. **Deploy:** Use the [Deployment Guide](guides/deployment.md) for production setup
3. **Configure:** Read the [Configuration Reference](reference/configuration.md)
4. **Build your plugin:** Follow the [Plugin Development Guide](guides/plugin-development.md)

## Audience

These documents target **Distributor engineering teams** who receive the
Chaperone source code and need to:

- Deploy the proxy in their infrastructure (Mode A — direct mTLS termination)
- Build a custom binary with their own credential-injection plugin
- Configure routing, TLS, and observability for production use
