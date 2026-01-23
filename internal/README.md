# Internal Packages

This directory contains private application code that is not intended to be imported by other projects.

## Packages

- `config/` - Configuration loading (YAML + environment variables)
- `proxy/` - Core reverse proxy logic, routing, and request lifecycle
- `cache/` - In-memory credential cache with memguard encryption
- `observability/` - Structured logging, Prometheus metrics, and tracing
