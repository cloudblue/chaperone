# Chaperone

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**Chaperone** is a high-performance, sidecar-style egress proxy designed to securely inject credentials into outgoing API requests. It runs within your infrastructure to manage sensitive authentication data (tokens, API keys, OAuth2 credentials) without exposing them to upstream platforms.

## Features

- 🔐 **Secure Credential Injection** - Credentials never leave your infrastructure
- 🔒 **Mutual TLS (mTLS)** - Full support for client certificate authentication
- ⚡ **High Performance** - Built in Go with efficient connection handling
- 🔌 **Plugin Architecture** - Extensible via static recompilation (no runtime dependencies)
- 📊 **Observable** - Structured logging and metrics support
- 🐳 **Cloud Native** - Designed for containerized deployments

## Project Status

🚧 **Work in Progress** - This project is currently in the Proof of Concept (PoC) phase.

## Requirements

- Go 1.21 or higher
- Docker (optional, for containerized deployment)

## Quick Start

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
├── cmd/chaperone/          # Main application entry point
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

### For Plugin Developers

```bash
# Import just the SDK in your project
go get github.com/cloudblue/chaperone/sdk
```

## Configuration

Chaperone is configured via a YAML file (`config.yaml`) with environment variable overrides following the 12-Factor App methodology.

```yaml
server:
  addr: ":443"

observability:
  log_level: "info"
```

Environment variables follow the pattern `CHAPERONE_<SECTION>_<KEY>`:
```bash
export CHAPERONE_SERVER_ADDR=":8443"
```

## Development

```bash
# Build for development (fast, with debug symbols)
make build-dev

# Build for production (stripped, with version info)
make build

# Run tests
make test

# Run tests with race detector
make test-race

# Run tests with coverage report
make test-cover

# Run linters
make lint

# Format code
make fmt

# Show all available commands
make help
```

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
