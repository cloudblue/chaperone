---
applyTo: "**/*.go"
---

# Go Security Conventions

These security-focused conventions apply to all Go code in Chaperone.
**Security is the primary concern** - when in doubt, choose the more secure option.

## Sensitive Data Handling

### Headers That MUST Be Redacted

```go
var sensitiveHeaders = []string{
    "Authorization",
    "Proxy-Authorization",
    "Cookie",
    "Set-Cookie",
    "X-API-Key",
    "X-Auth-Token",
}
```

### Never Log Credentials

```go
// ✅ Correct: Redact sensitive headers
func logRequest(req *http.Request) {
    headers := make(http.Header)
    for k, v := range req.Header {
        if isSensitive(k) {
            headers[k] = []string{"[REDACTED]"}
        } else {
            headers[k] = v
        }
    }
    slog.Info("request", "headers", headers)
}

// ❌ Never: Log raw headers
slog.Info("request", "headers", req.Header)
```

### Memory Protection for Secrets

```go
// ✅ Correct: Use memguard for in-memory credentials
import "github.com/awnumar/memguard"

secret := memguard.NewBufferFromBytes([]byte(token))
defer secret.Destroy()

// ❌ Never: Plain strings for long-lived credentials
var cachedToken string = "secret-token"
```

## Input Validation

### Target URL Validation

```go
// ✅ Correct: Validate against allow-list BEFORE use
func handleRequest(targetURL string, allowList AllowList) error {
    if err := validateTargetURL(targetURL, allowList); err != nil {
        return fmt.Errorf("target validation failed: %w", err)
    }
    // proceed with request
}
```

### Sanitize Error Messages

```go
// ✅ Correct: Generic error to caller, detailed log internally
if err := authenticate(creds); err != nil {
    slog.Error("auth failed", "vendor", vendorID, "error", err)
    return ErrAuthenticationFailed // Generic
}

// ❌ Never: Expose internal details
return fmt.Errorf("auth failed: wrong password for user %s", username)
```

## TLS Configuration

### Minimum TLS Version

```go
// ✅ Correct: TLS 1.3 minimum
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
}

// ❌ Never: Allow older TLS versions in production
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS10, // Vulnerable!
}
```

### Certificate Validation

```go
// ✅ Correct: Always verify certificates
tlsConfig := &tls.Config{
    InsecureSkipVerify: false, // Default, but be explicit
}

// ❌ Never: Skip verification (except in tests with comment)
tlsConfig := &tls.Config{
    InsecureSkipVerify: true, // ONLY in test with mock server
}
```

## Response Sanitization

### Strip Auth Headers from Responses

```go
// ✅ Correct: Always sanitize before returning upstream
func sanitizeResponse(resp *http.Response) {
    for _, h := range sensitiveHeaders {
        resp.Header.Del(h)
    }
}
```

### Error Masking

```go
// ✅ Correct: Mask upstream errors
func handleUpstreamError(resp *http.Response) *http.Response {
    if resp.StatusCode >= 400 {
        // Log original for debugging
        slog.Warn("upstream error", "status", resp.StatusCode)
        
        // Return generic error
        return genericErrorResponse(resp.StatusCode)
    }
    return resp
}
```

## Panic Recovery

```go
// ✅ Correct: Recover from panics in handlers
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    defer func() {
        if err := recover(); err != nil {
            slog.Error("panic recovered", "error", err, "stack", debug.Stack())
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        }
    }()
    // handler code
}
```

## Timeouts

```go
// ✅ Correct: Always set timeouts
client := &http.Client{
    Timeout: 30 * time.Second,
}

server := &http.Server{
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    IdleTimeout:  120 * time.Second,
}

// ❌ Never: No timeout (allows resource exhaustion)
client := &http.Client{}
```

## Security Testing Requirements

Every security-sensitive function MUST have tests for:
1. Valid input (happy path)
2. Invalid/malformed input
3. Boundary conditions
4. Attempted bypass (e.g., path traversal, header injection)
