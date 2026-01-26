# Task: mTLS Verification

**Status:** [ ] Not Started  
**Priority:** P0  
**Estimated Effort:** L (Large)

## Objective

Verify that mutual TLS (mTLS) handshake works correctly using Go's `httptest` package with client certificates.

## Design Spec Reference

- **Primary:** Section 5.3 - Security Controls (Authentication: mTLS mandatory)
- **Primary:** Section 8.2 - Deployment & mTLS Enrollment
- **Primary:** Section 6.1 - Mode A: Standalone/Direct
- **Related:** Section 9.1.B - Integration Testing (Mock World)
- **Related:** Section 9.2.A - Negative Testing

## Dependencies

- [x] `08-core-skeleton.task.md` - Core skeleton with HTTP server

## Acceptance Criteria

- [ ] Server configured with TLS 1.3 minimum
- [ ] Server requires client certificate
- [ ] Server validates client certificate against CA
- [ ] Test scenarios pass:
  - [ ] Valid client cert → 200 OK
  - [ ] No client cert → Connection rejected
  - [ ] Invalid client cert (wrong CA) → Connection rejected
  - [ ] Expired client cert → Connection rejected
- [ ] Tests use in-memory certs (no files for tests)
- [ ] Tests pass: `go test ./internal/proxy/... -run TestMTLS`

## Implementation Hints

### TLS Configuration

```go
func (s *Server) configureTLS(caCert, serverCert, serverKey []byte) *tls.Config {
    // Load CA cert pool
    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)
    
    // Load server cert
    cert, err := tls.X509KeyPair(serverCert, serverKey)
    if err != nil {
        return nil, err
    }
    
    return &tls.Config{
        Certificates: []tls.Certificate{cert},
        ClientAuth:   tls.RequireAndVerifyClientCert,
        ClientCAs:    caCertPool,
        MinVersion:   tls.VersionTLS13,
    }
}
```

### Test Cert Generation

Generate test certificates in memory using `crypto/x509`:

```go
func generateTestCA() (certPEM, keyPEM []byte, err error) {
    priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    
    template := x509.Certificate{
        SerialNumber: big.NewInt(1),
        Subject:      pkix.Name{CommonName: "Test CA"},
        NotBefore:    time.Now(),
        NotAfter:     time.Now().Add(time.Hour),
        IsCA:         true,
        KeyUsage:     x509.KeyUsageCertSign,
    }
    
    certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
    
    // PEM encode...
    return certPEM, keyPEM, nil
}

func generateClientCert(caCert, caKey []byte) (certPEM, keyPEM []byte, err error) {
    // Similar, but signed by CA
}
```

### Test Structure

```go
func TestMTLS_ValidClientCert_Success(t *testing.T) {
    // Generate test CA
    caCert, caKey := generateTestCA()
    
    // Generate server cert signed by CA
    serverCert, serverKey := generateCert(caCert, caKey, "server")
    
    // Generate client cert signed by same CA
    clientCert, clientKey := generateCert(caCert, caKey, "client")
    
    // Start server with mTLS
    server := httptest.NewUnstartedServer(handler)
    server.TLS = configureTLS(caCert, serverCert, serverKey)
    server.StartTLS()
    defer server.Close()
    
    // Create client with cert
    client := createTLSClient(caCert, clientCert, clientKey)
    
    // Make request
    resp, err := client.Get(server.URL + "/_ops/health")
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Errorf("expected 200, got %d", resp.StatusCode)
    }
}

func TestMTLS_NoClientCert_Rejected(t *testing.T) {
    // Similar setup, but client without cert
    // Expect connection error
}

func TestMTLS_WrongCA_Rejected(t *testing.T) {
    // Client cert signed by different CA
    // Expect connection error
}

func TestMTLS_ExpiredCert_Rejected(t *testing.T) {
    // Client cert with NotAfter in past
    // Expect connection error
}
```

### Gotchas

- `httptest.NewTLSServer` uses self-signed by default; use `NewUnstartedServer` + manual TLS
- TLS errors often surface as connection errors, not HTTP errors
- Certificate generation can be slow; generate once per test file
- Don't forget to handle both server and client cert validation

## Files to Create/Modify

- [ ] `internal/proxy/tls.go` - TLS configuration
- [ ] `internal/proxy/mtls_test.go` - mTLS tests
- [ ] `internal/testutil/certs.go` - Test certificate generation helpers

## Testing Strategy

### Positive Tests

| Scenario | Expected |
|----------|----------|
| Valid client cert | 200 OK |
| Client cert with different CN | 200 OK (CN not validated in PoC) |

### Negative Tests

| Scenario | Expected |
|----------|----------|
| No client cert | Connection refused |
| Self-signed client cert | Connection refused |
| Cert from different CA | Connection refused |
| Expired client cert | Connection refused |
| TLS 1.2 client | Connection refused (TLS 1.3 min) |

## Security Considerations

- TLS 1.3 minimum (per Design Spec)
- Certificate validation is mandatory
- Test certs should have short expiry (1 hour for tests)
- Never use test certs in production

## Notes

This task validates Mode A deployment where the proxy terminates mTLS directly.
Mode B (behind reverse proxy) is Phase 4 scope.
