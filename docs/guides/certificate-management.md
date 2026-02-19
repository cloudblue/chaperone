# Certificate Management

How to generate, enroll, and manage TLS certificates for Chaperone.
Covers development self-signed certificates and production CA enrollment.

## How mTLS Works in Chaperone

Chaperone uses mutual TLS (mTLS) to authenticate both sides of the
connection between the platform and your proxy:

```
Platform (Connector)                          Your Proxy (Chaperone)
        │                                              │
        │──── presents client certificate ────────────▸│ verifies against CA cert
        │◂──── presents server certificate ────────────│
        │  verifies against platform CA                │
        │                                              │
        └──────────── encrypted channel ───────────────┘
```

This means your proxy needs **three certificate files**:

| File | What it is | Who provides it |
|------|-----------|-----------------|
| `server.key` | Your proxy's private key | **You generate it** (via `chaperone enroll`) |
| `server.crt` | Your proxy's signed certificate | **Your CA signs it** (from the CSR you submit) |
| `ca.crt` | The CA that signed the platform's client certificates | **The platform provides it** |

These map directly to the config fields in
[`server.tls`](../reference/configuration.md):

```yaml
tls:
  cert_file: "/app/certs/server.crt"   # Your signed certificate
  key_file:  "/app/certs/server.key"   # Your private key
  ca_file:   "/app/certs/ca.crt"       # Platform's CA (for client verification)
```

### Certificate Requirements

- **Key type:** ECDSA P-256 (generated automatically by `chaperone enroll`)
- **TLS version:** 1.3 minimum (enforced by Chaperone)
- **SANs:** The server certificate must include the DNS names and/or IP
  addresses that clients use to reach your proxy

> **RSA certificates are not supported.** Chaperone generates and expects
> ECDSA P-256 keys. If your CA requires a specific key type, check with
> them before enrolling.

## How to Generate Development Certificates

For local development and testing, generate self-signed certificates:

```bash
make gencerts
```

This creates a `certs/` directory with all the files needed for local
mTLS testing:

| File | Purpose |
|------|---------|
| `ca.crt` | Test CA certificate |
| `server.crt` / `server.key` | Proxy's server identity |
| `client.crt` / `client.key` | **Simulates the platform** — use with `curl --cert` to test mTLS locally |

In production, you only generate `server.key` and `server.csr` (via
`chaperone enroll`). The platform provides its own client certificates
and CA — you never create those yourself.

To customize the server certificate SANs:

```bash
make gencerts DOMAINS="myserver.local,192.168.1.100"
```

## How to Enroll with a Production CA

For production deployments, generate a CSR and submit it to your
Certificate Authority (Connect Portal, HashiCorp Vault, internal PKI,
etc.).

### Step 1: Generate a Key Pair and CSR

```bash
# Single domain
./chaperone enroll --domains proxy.example.com

# Multiple domains and IPs
./chaperone enroll --domains proxy.example.com,10.0.0.1 --cn my-proxy

# Custom output directory
./chaperone enroll --domains proxy.example.com --out /etc/chaperone/certs
```

This generates `server.key` (your private key — keep it safe) and
`server.csr` (the Certificate Signing Request to submit to your CA).

> **Using the Go API?** See the
> [Plugin Development Guide](plugin-development.md#adding-enrollment-support)
> for wiring `chaperone.Enroll()` into your custom binary, and the
> [SDK Reference](../reference/sdk.md#enrollconfig) for all
> `EnrollConfig` fields.

### Step 2: Submit the CSR to Your CA

Send `server.csr` to your Certificate Authority through their standard
process. This varies by organization — it might be a web portal, an API
call, or a ticket to your security team.

### Step 3: Install the Signed Certificate

Once you receive the signed `server.crt`:

1. Place `server.crt` alongside `server.key` in your certs directory
2. Place the platform's `ca.crt` in the same directory (provided by the
   platform during onboarding)
3. Update your `config.yaml` to point to these files (see
   [Configuration Reference](../reference/configuration.md))
4. Start (or restart) Chaperone — it loads certificates on boot

## How to Verify Certificates

Check certificate details and validity:

```bash
# Verify the client certificate was signed by the expected CA
openssl verify -CAfile certs/ca.crt certs/client.crt

# Check server certificate details (issuer, SANs)
openssl x509 -in certs/server.crt -text -noout | grep -A2 "Issuer"

# Check certificate expiry dates
openssl x509 -in certs/server.crt -noout -dates
# notBefore=...
# notAfter=...
```

## How to Renew Certificates

For development, regenerate:

```bash
make gencerts
```

For production, repeat the enrollment process — generate a new CSR and
submit it to your CA:

```bash
./chaperone enroll --domains your.domain.com
# Submit the new server.csr to your CA, install the new server.crt
```

> **Automatic rotation** via `CertificateSigner.SignCSR()` is planned for
> a future release. See the
> [Design Specification](../explanation/DESIGN-SPECIFICATION.md) for
> details.

## Troubleshooting

For mTLS connection issues (LibreSSL errors, chain validation failures,
expired certificates), see the [Troubleshooting Guide](troubleshooting.md#mtls-issues).

## Next Steps

- [Deployment Guide](deployment.md) — Deploy with Docker or Kubernetes
- [Configuration Reference](../reference/configuration.md) — TLS configuration options
