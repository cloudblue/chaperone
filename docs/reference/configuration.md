# Configuration Reference

Complete reference for all Chaperone configuration options. Chaperone uses
YAML configuration with environment variable overrides following the
[12-Factor App](https://12factor.net/config) methodology.

## Configuration File Resolution

Chaperone resolves the configuration file path in this order:

1. **`-config` flag** — `chaperone -config /etc/chaperone.yaml`
2. **`CHAPERONE_CONFIG` environment variable** — `export CHAPERONE_CONFIG=/etc/chaperone.yaml`
3. **`./config.yaml`** — current working directory (default)

If no configuration file is found, Chaperone starts with built-in defaults.

## Environment Variable Overrides

Environment variables follow the pattern:

```
CHAPERONE_<SECTION>_<KEY>
```

Rules:
- All uppercase, underscore separator
- Nested keys use underscore joining: `server.tls.cert_file` → `CHAPERONE_SERVER_TLS_CERT_FILE`
- Environment variables **always override** YAML values
- Duration values use Go syntax: `5s`, `30s`, `2m`, `1h`

```bash
# Examples
export CHAPERONE_SERVER_ADDR=":9443"
export CHAPERONE_OBSERVABILITY_LOG_LEVEL="debug"
export CHAPERONE_SERVER_TLS_CERT_FILE="/etc/certs/server.crt"
export CHAPERONE_UPSTREAM_TIMEOUTS_CONNECT="10s"
```

## Configuration Sections

### Server

Controls the proxy and admin server listeners.

```yaml
server:
  addr: ":443"
  admin_addr: "127.0.0.1:9090"
  shutdown_timeout: 30s
  tls:
    enabled: true
    cert_file: "/certs/server.crt"
    key_file: "/certs/server.key"
    ca_file: "/certs/ca.crt"
    auto_rotate: true
```

| Key | Env Override | Type | Default | Description |
|-----|-------------|------|---------|-------------|
| `addr` | `CHAPERONE_SERVER_ADDR` | string | `:443` | Traffic port address |
| `admin_addr` | `CHAPERONE_SERVER_ADMIN_ADDR` | string | `127.0.0.1:9090` | Admin/metrics port (bound to localhost for security) |
| `shutdown_timeout` | `CHAPERONE_SERVER_SHUTDOWN_TIMEOUT` | duration | `30s` | Max time to drain in-flight requests during graceful shutdown |

#### TLS Sub-section

| Key | Env Override | Type | Default | Description |
|-----|-------------|------|---------|-------------|
| `tls.enabled` | `CHAPERONE_SERVER_TLS_ENABLED` | bool | `true` | Enable TLS/mTLS (production builds enforce HTTPS-only targets) |
| `tls.cert_file` | `CHAPERONE_SERVER_TLS_CERT_FILE` | string | `/certs/server.crt` | Path to PEM-encoded server certificate |
| `tls.key_file` | `CHAPERONE_SERVER_TLS_KEY_FILE` | string | `/certs/server.key` | Path to PEM-encoded server private key |
| `tls.ca_file` | `CHAPERONE_SERVER_TLS_CA_FILE` | string | `/certs/ca.crt` | Path to PEM-encoded CA certificate for client verification |
| `tls.auto_rotate` | `CHAPERONE_SERVER_TLS_AUTO_ROTATE` | bool | `true` | Enable automatic certificate rotation (Phase 3) |

### Upstream

Controls target request routing, context header parsing, and timeouts.

```yaml
upstream:
  header_prefix: "X-Connect"
  trace_header: "Connect-Request-ID"
  allow_list:
    "api.vendor.com":
      - "/v1/**"
      - "/v2/**"
    "payments.example.com":
      - "/api/charge"
      - "/api/refund"
  timeouts:
    connect: 5s
    read: 30s
    write: 30s
    idle: 120s
    keep_alive: 30s
    plugin: 10s
```

| Key | Env Override | Type | Default | Description |
|-----|-------------|------|---------|-------------|
| `header_prefix` | `CHAPERONE_UPSTREAM_HEADER_PREFIX` | string | `X-Connect` | Prefix for context headers (configurable per ADR-005) |
| `trace_header` | `CHAPERONE_UPSTREAM_TRACE_HEADER` | string | `Connect-Request-ID` | Correlation ID header name |
| `allow_list` | — | map | **Required** | Host → path patterns (see [Allow-List Syntax](#allow-list-syntax)) |

#### Timeouts

| Key | Env Override | Type | Default | Description |
|-----|-------------|------|---------|-------------|
| `timeouts.connect` | `CHAPERONE_UPSTREAM_TIMEOUTS_CONNECT` | duration | `5s` | TCP connection establishment timeout to vendor API |
| `timeouts.read` | `CHAPERONE_UPSTREAM_TIMEOUTS_READ` | duration | `30s` | Time to read response headers from vendor |
| `timeouts.write` | `CHAPERONE_UPSTREAM_TIMEOUTS_WRITE` | duration | `30s` | Time to write request body to vendor |
| `timeouts.idle` | `CHAPERONE_UPSTREAM_TIMEOUTS_IDLE` | duration | `120s` | Keep-alive connection idle timeout |
| `timeouts.keep_alive` | `CHAPERONE_UPSTREAM_TIMEOUTS_KEEP_ALIVE` | duration | `30s` | TCP keep-alive probe interval |
| `timeouts.plugin` | `CHAPERONE_UPSTREAM_TIMEOUTS_PLUGIN` | duration | `10s` | Maximum time for plugin `GetCredentials` call |

### Observability

Controls logging, profiling, and header redaction.

```yaml
observability:
  log_level: "info"
  enable_profiling: false
  sensitive_headers:
    - "X-Custom-Secret"
    - "X-Vendor-Token"
```

| Key | Env Override | Type | Default | Description |
|-----|-------------|------|---------|-------------|
| `log_level` | `CHAPERONE_OBSERVABILITY_LOG_LEVEL` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `enable_profiling` | `CHAPERONE_OBSERVABILITY_ENABLE_PROFILING` | bool | `false` | Enable `/debug/pprof` endpoints on the admin port |
| `sensitive_headers` | — | []string | See below | Additional headers to redact (merged with defaults) |

#### Sensitive Headers

The following headers are **always** redacted in logs, regardless of configuration:

- `Authorization`
- `Proxy-Authorization`
- `Cookie`
- `Set-Cookie`
- `X-API-Key`
- `X-Auth-Token`

Any headers listed in `sensitive_headers` are **merged** with these defaults.
You cannot accidentally remove the built-in list by specifying custom headers —
this is a security safeguard.

```yaml
# These are ADDED to the defaults, not replacing them
observability:
  sensitive_headers:
    - "X-Custom-Secret"
    - "X-Vendor-Token"
```

## Allow-List Syntax

The allow-list enforces a **default-deny** policy. Only requests matching
a host+path combination are forwarded to the vendor API. All other requests
receive a `403 Forbidden` response.

### Pattern Rules

| Pattern | Matches | Example |
|---------|---------|---------|
| `/exact/path` | Exact path only | `/api/charge` |
| `*` | Single path segment (within one `/`) | `/v1/*/info` matches `/v1/users/info` but not `/v1/a/b/info` |
| `**` | Zero or more path segments (recursive) | `/v1/**` matches `/v1/`, `/v1/users`, `/v1/users/123/orders` |

### Examples

```yaml
upstream:
  allow_list:
    # Exact host, recursive wildcard
    "api.vendor.com":
      - "/v1/**"            # All paths under /v1/
      - "/v2/products/*"    # Single level under /v2/products/

    # Exact paths only
    "payments.example.com":
      - "/api/charge"       # Only /api/charge (exact)
      - "/api/refund"       # Only /api/refund (exact)

    # Mixed patterns
    "storage.vendor.io":
      - "/buckets/*/objects/**"  # Objects within any bucket
```

### Debugging Allow-List Denials

If a request is denied (403), check:

1. The host in `X-Connect-Target-URL` matches an allow-list key exactly (case-sensitive)
2. The path matches one of the patterns for that host
3. There are no trailing slashes causing mismatches

Use `debug` log level to see detailed allow-list evaluation:

```bash
export CHAPERONE_OBSERVABILITY_LOG_LEVEL="debug"
```

## Timeout Tuning Guidance

### When to Adjust Timeouts

| Scenario | Recommended Change |
|----------|--------------------|
| Slow vendor APIs (>10s response time) | Increase `read` to 60–120s |
| Large file uploads | Increase `write` to 120s+ |
| Plugin calls external secrets manager (Vault, AWS Secrets Manager) | Increase `plugin` to 15–30s |
| High connection churn | Decrease `idle` to 60s |
| Unstable network | Decrease `connect` to 3s, increase `keep_alive` to 10s |
| Graceful rolling deployment | Set `shutdown_timeout` ≥ max expected request duration |

### Performance Impact

- **Lower `connect`** = Fail fast on unreachable hosts, better user experience
- **Higher `read`/`write`** = Tolerate slow vendors, but hold connections longer
- **Lower `idle`** = Free resources faster, but more TCP handshakes
- **Higher `plugin`** = Tolerate slow credential providers, but block request longer
- **`shutdown_timeout`** should be ≥ longest expected in-flight request

### Example: High-Throughput Deployment

```yaml
upstream:
  timeouts:
    connect: 3s        # Fail fast
    read: 60s          # Vendor has slow endpoints
    write: 60s         # Large payloads
    idle: 60s          # Reclaim connections aggressively
    keep_alive: 15s    # Frequent probes
    plugin: 15s        # Vault calls may be slow
```

## Complete Annotated Example

The file `configs/config.example.yaml` contains a fully annotated configuration:

```yaml
server:
  addr: ":8443"               # Traffic port
  admin_addr: ":9090"         # Admin port (health, metrics, pprof)
  shutdown_timeout: 30s       # Graceful shutdown drain time
  tls:
    enabled: true             # Enable mTLS
    cert_file: "certs/server.crt"
    key_file: "certs/server.key"
    ca_file: "certs/ca.crt"
    auto_rotate: true         # Phase 3: automatic cert rotation

upstream:
  header_prefix: "X-Connect"           # Context header prefix (ADR-005)
  trace_header: "Connect-Request-ID"   # Correlation ID header
  allow_list:                           # Default-deny: only listed hosts/paths
    "api.vendor.com":
      - "/v1/**"
      - "/v2/**"
    "payments.example.com":
      - "/api/charge"
      - "/api/refund"
  timeouts:
    connect: 5s       # TCP dial timeout
    read: 30s         # Response header timeout
    write: 30s        # Response write timeout
    idle: 120s        # Keep-alive connection idle timeout
    keep_alive: 30s   # TCP keep-alive probe interval
    plugin: 10s       # Max time for plugin GetCredentials

observability:
  log_level: "info"            # debug | info | warn | error
  enable_profiling: false      # Enable /debug/pprof on admin port
  sensitive_headers:           # Additional headers to redact (merged with defaults)
    # - "X-Custom-Secret"
    # - "X-Vendor-Token"
```

## Next Steps

- [Deployment Guide](../guides/deployment.md) — Deploy Chaperone in your environment
- [Plugin Development Guide](../guides/plugin-development.md) — Build your credential plugin
- [Troubleshooting](../guides/troubleshooting.md) — Common issues and solutions
