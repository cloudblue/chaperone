# Certificate Management

How to generate, enroll, and manage TLS certificates for Chaperone.
Covers development self-signed certificates and production CA enrollment.

## Development Certificates

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

## Production Certificates (CA Enrollment)

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

**`EnrollConfig` fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Domains` | `string` | — (required) | Comma-separated DNS names and IPs for SANs |
| `CommonName` | `string` | `"chaperone"` | Certificate Common Name |
| `OutputDir` | `string` | `"certs"` | Directory for output files (created if absent) |

**`EnrollResult` fields:**

| Field | Type | Description |
|-------|------|-------------|
| `KeyFile` | `string` | Path to the generated ECDSA P-256 private key |
| `CSRFile` | `string` | Path to the generated Certificate Signing Request |
| `DNSNames` | `[]string` | DNS SANs included in the CSR |
| `IPs` | `[]net.IP` | IP SANs included in the CSR |

### Wiring the `enroll` Subcommand in Your Binary

Add enrollment support to your Distributor binary with a simple subcommand
check in `main.go`:

```go
func main() {
    if len(os.Args) > 1 && os.Args[1] == "enroll" {
        if err := runEnroll(); err != nil {
            fmt.Fprintf(os.Stderr, "enrollment failed: %v\n", err)
            os.Exit(1)
        }
        return
    }

    // Normal proxy startup...
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := chaperone.Run(ctx, myPlugin, /* options */); err != nil {
        os.Exit(1)
    }
}

func runEnroll() error {
    result, err := chaperone.Enroll(context.Background(), chaperone.EnrollConfig{
        Domains:    "proxy.example.com",
        OutputDir:  "./certs",
    })
    if err != nil {
        return err
    }
    fmt.Printf("Key:  %s\n", result.KeyFile)
    fmt.Printf("CSR:  %s\n", result.CSRFile)
    return nil
}
```

Then run:

```bash
./my-proxy enroll --domains proxy.example.com
```

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
