# Certificate Management

How to generate, enroll, and manage TLS certificates for Chaperone.
Covers development self-signed certificates and production CA enrollment.

## Generate Development Certificates

For local development and testing, generate self-signed certificates:

```bash
make gencerts
```

This creates a `certs/` directory with a test CA, server certificate, and
client certificate — all ECDSA P-256.

To customize the server certificate SANs:

```bash
make gencerts DOMAINS="myserver.local,192.168.1.100"
```

## Enroll with a Production CA

For production deployments, generate a CSR and submit it to your Certificate
Authority (Connect Portal, HashiCorp Vault, internal PKI, etc.).

### Using the CLI

```bash
# Single domain
./chaperone enroll --domains proxy.example.com

# Multiple domains and IPs
./chaperone enroll --domains proxy.example.com,10.0.0.1 --cn my-proxy

# Custom output directory
./chaperone enroll --domains proxy.example.com --out /etc/chaperone/certs
```

### Using the Public API

If you're building a custom binary with the "Own Repo" workflow, use
`chaperone.Enroll()` in your `main.go`:

```go
result, err := chaperone.Enroll(context.Background(), chaperone.EnrollConfig{
    Domains:    "proxy.example.com,10.0.0.1",
    CommonName: "my-proxy",
    OutputDir:  "./certs",
})
if err != nil {
    log.Fatalf("enrollment failed: %v", err)
}
fmt.Printf("Key:  %s\n", result.KeyFile)
fmt.Printf("CSR:  %s\n", result.CSRFile)
```

See the SDK Reference for all
[`EnrollConfig`](../reference/sdk.md#enrollconfig) fields (including
`Force` for overwriting existing files) and
[`EnrollResult`](../reference/sdk.md#enrollresult) fields.

### Wiring the `enroll` Subcommand in Your Binary

To add enrollment as a subcommand in your custom binary, see the
[Plugin Development Guide](plugin-development.md#adding-enrollment-support)
for a complete `main.go` example.

### Enrollment Workflow

1. Run `chaperone enroll --domains your.domain.com` (or call `chaperone.Enroll()`)
2. Submit the generated `server.csr` to your CA
3. Place the signed `server.crt` alongside `server.key` in your certs directory
4. Start Chaperone — it will load the certificate on boot

## How to Verify Certificates

Check certificate details and validity:

```bash
# Verify the certificate chain
openssl verify -CAfile certs/ca.crt certs/client.crt

# Check certificate details
openssl x509 -in certs/server.crt -text -noout | grep -A2 "Issuer"
openssl x509 -in certs/ca.crt -text -noout | grep -A2 "Subject"

# Check certificate expiry
openssl x509 -in certs/server.crt -noout -dates
# notBefore=...
# notAfter=...
```

## How to Renew Expired Certificates

For development, regenerate certificates:

```bash
make gencerts
```

For production, re-enroll with your CA:

```bash
./chaperone enroll --domains your.domain.com
# Submit the new CSR to your CA
```

## Troubleshooting

For mTLS connection issues (LibreSSL errors, chain validation failures,
expired certificates), see the [Troubleshooting Guide](troubleshooting.md#mtls-issues).

## Next Steps

- [Deployment Guide](deployment.md) — Deploy Chaperone with Docker or Kubernetes
- [Configuration Reference](../reference/configuration.md) — TLS configuration options
