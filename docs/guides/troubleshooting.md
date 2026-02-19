# Troubleshooting

Common issues and solutions when deploying and operating Chaperone.

## Common Startup Errors

### Missing Configuration File

```
Error: loading configuration: accessing config file ./config.yaml: stat ./config.yaml: no such file or directory
```

**Solution:** Ensure a configuration file exists at one of these locations
(checked in order):

1. Path specified via `-config` flag
2. Path in `CHAPERONE_CONFIG` environment variable
3. `./config.yaml` in the current directory

```bash
# Verify the file exists
ls -la config.yaml

# Or specify explicitly
./chaperone -config /path/to/config.yaml
```

### TLS Certificate Errors

```
Error: loading configuration: configuration validation failed: server.tls.cert_file: TLS file not found: /certs/server.crt
```

**Solution:** Ensure your certificate files exist at the paths specified
in your configuration:

```bash
# Check certificate paths
ls -la certs/server.crt certs/server.key certs/ca.crt

# For development, generate test certificates
make gencerts

# For production, run enrollment
./chaperone enroll --domains your.domain.com
```

### Port Already in Use

```
Error: server error: listen tcp :8443: bind: address already in use
```

**Solution:** Another process is using port 8443 (or your configured port).

```bash
# Find the process using the port
lsof -i :8443

# Or change the port in your config
# config.yaml
server:
  addr: ":9443"
```

### Invalid Configuration Values

```
Error: loading configuration: configuration validation failed: server.shutdown_timeout: timeout must be positive
```

**Solution:** Check your configuration for invalid values. Duration values
use Go syntax (`5s`, `30s`, `2m`). See the
[Configuration Reference](../reference/configuration.md) for valid ranges and defaults.

## mTLS Issues

### macOS LibreSSL + ECDSA Workaround

The built-in `curl` on macOS uses LibreSSL, which may fail with ECDSA
client certificates:

```
curl: (58) unable to set private key file: 'certs/client.key' type PEM
```

**Solution:** Use the Docker-based curl image:

```bash
docker run --rm --network host -v $(pwd)/certs:/certs:ro curlimages/curl \
    --cacert /certs/ca.crt \
    --cert /certs/client.crt \
    --key /certs/client.key \
    https://localhost:8443/_ops/health
```

Or install curl with OpenSSL via Homebrew:

```bash
brew install curl
# Use the Homebrew curl (check path with: brew --prefix curl)
/opt/homebrew/opt/curl/bin/curl --cacert certs/ca.crt \
    --cert certs/client.crt \
    --key certs/client.key \
    https://localhost:8443/_ops/health
```

### Certificate Chain Validation Errors

```
Error: tls: failed to verify certificate: x509: certificate signed by unknown authority
```

**Solution:** Ensure the CA certificate (`ca.crt`) used by the server
matches the CA that signed the client certificate:

```bash
# Verify the certificate chain
openssl verify -CAfile certs/ca.crt certs/client.crt

# Check certificate details
openssl x509 -in certs/server.crt -text -noout | grep -A2 "Issuer"
openssl x509 -in certs/ca.crt -text -noout | grep -A2 "Subject"
```

### Expired Certificates

```
Error: tls: internal error (or: x509: certificate has expired)
```

**Solution:** Check certificate expiry:

```bash
openssl x509 -in certs/server.crt -noout -dates
# notBefore=...
# notAfter=...
```

For development, regenerate certificates:

```bash
make gencerts
```

For production, re-enroll with your CA:

```bash
./chaperone enroll --domains your.domain.com
# Submit the new CSR to your CA
```

## Allow-List Denials

### 403 Forbidden Responses

```json
{"error": "host not allowed"}
```

**Possible causes:**

1. **Host not in allow-list** — The target URL's host is not listed in
   `upstream.allow_list`.

2. **Path not matched** — The path doesn't match any pattern for the host.

3. **Case sensitivity** — Host matching is case-sensitive. Ensure the host
   in `X-Connect-Target-URL` matches the allow-list key exactly.

**Debugging steps:**

```bash
# Enable debug logging to see allow-list evaluation
export CHAPERONE_OBSERVABILITY_LOG_LEVEL="debug"

# Restart the proxy and retry the request
# Look for log entries about allow-list matching
```

**Common pattern mistakes:**

```yaml
# ❌ Missing recursive wildcard — only matches /v1/ exactly
allow_list:
  "api.vendor.com":
    - "/v1/"

# ✅ Recursive wildcard — matches all paths under /v1/
allow_list:
  "api.vendor.com":
    - "/v1/**"

# ❌ Trailing slash mismatch
allow_list:
  "api.vendor.com":
    - "/api/charge/"    # Won't match /api/charge (no trailing slash)

# ✅ Exact path without trailing slash
allow_list:
  "api.vendor.com":
    - "/api/charge"
```

### Missing Target URL Header

```json
{"error": "missing Target-URL header"}
```

**Solution:** Ensure the request includes the `X-Connect-Target-URL`
header (or your configured `header_prefix` + `-Target-URL`):

```bash
curl -H "X-Connect-Target-URL: https://api.vendor.com/v1/test" \
     https://localhost:8443/proxy
```

## Plugin Errors

### Plugin Timeout

The server logs `"plugin timeout"` at ERROR level and returns an HTTP `504 Gateway Timeout` response with body `Gateway Timeout`.

**Solution:** Your plugin's `GetCredentials` call is taking too long.

1. **Increase the plugin timeout** if your credential source is legitimately slow:

```yaml
upstream:
  timeouts:
    plugin: 30s
```

2. **Optimize your plugin** — check for network latency to your credential
   source (Vault, database, OAuth2 provider).

3. **Use context** — ensure your plugin respects the `ctx` parameter for
   cancellation:

```go
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    // Use ctx for HTTP calls — cancellation is automatic
    vaultReq, _ := http.NewRequestWithContext(ctx, "GET", p.vaultURL, nil)
    resp, err := p.httpClient.Do(vaultReq)
    // ...
}
```

### Plugin Panic Recovery

If your plugin panics, Chaperone's recovery middleware catches it and
returns a `500 Internal Server Error` response. The panic is logged:

```
level=ERROR msg="panic recovered" error="runtime error: index out of range"
```

**Solution:** Fix the panic in your plugin code. Check for nil pointer
dereferences, out-of-range indexing, and map access on nil maps.

### Credential Format Errors

If `GetCredentials` returns an invalid `Credential` (e.g., empty headers),
the request is forwarded without credential injection. Check your plugin's
return values:

```go
// ✅ Valid credential
return &sdk.Credential{
    Headers:   map[string]string{"Authorization": "Bearer " + token},
    ExpiresAt: time.Now().Add(1 * time.Hour),
}, nil

// ❌ Empty headers — no credential injection
return &sdk.Credential{
    Headers: map[string]string{},
}, nil
```

## Docker-Specific Issues

### Permission Denied on Certificate Files

```
Error: loading TLS certificates: open /app/certs/server.key: permission denied
```

**Solution:** The container runs as `nonroot` (UID 65532). Ensure
certificate files are readable:

```bash
# On the host, make files readable
chmod 644 certs/server.crt certs/ca.crt
chmod 600 certs/server.key

# Mount as read-only
docker run -v $(pwd)/certs:/app/certs:ro ...
```

### Container Networking

If the proxy can't reach vendor APIs, debug from the host (the
distroless container has no shell or network tools):

```bash
# Verify DNS and connectivity from the host
curl -I https://api.vendor.com/v1/health

# Check Docker network settings
docker inspect chaperone-proxy | grep -A5 "NetworkSettings"
```

### Image Size

The production image should be ~50 MB. If it's significantly larger:

```bash
# Check image size
docker images chaperone

# Verify multi-stage build is working (no build tools in final image)
docker history chaperone:latest
```

## Diagnostic Tools

Quick reference for diagnostic endpoints. See the
[HTTP API Reference](../reference/http-api.md) for full documentation.

```bash
# Health check
curl -s http://localhost:9090/_ops/health

# Version
curl -s http://localhost:9090/_ops/version

# Prometheus metrics
curl -s http://localhost:9090/metrics | head -20

# CPU profile (requires observability.enable_profiling: true)
go tool pprof http://localhost:9090/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:9090/debug/pprof/heap

# Goroutine dump
curl http://localhost:9090/debug/pprof/goroutine?debug=2
```

### Debug Logging

Enable verbose logging to diagnose issues:

```bash
export CHAPERONE_OBSERVABILITY_LOG_LEVEL="debug"
```

> **Warning:** Debug logging may produce high volume output. Use only for
> troubleshooting, not in production.

## Next Steps

- [Deployment Guide](deployment.md) — Deployment and operations
- [Configuration Reference](../reference/configuration.md) — Full configuration specification
- [Plugin Development Guide](plugin-development.md) — Build your credential plugin
