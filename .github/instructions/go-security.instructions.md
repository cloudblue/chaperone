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

### Security Default Lists: Always Merge, Never Replace

When configuration allows users to extend a security-critical list (e.g.,
`sensitive_headers`), the built-in defaults MUST always be included.
User entries are **merged on top**, never used as a replacement.

This prevents silent credential leaks when a Distributor adds custom
headers without realizing the defaults disappear.

```go
// ✅ Correct: Merge user entries with mandatory defaults
func applyDefaults(cfg *Config) {
    cfg.SensitiveHeaders = MergeSensitiveHeaders(
        cfg.SensitiveHeaders, // built-in defaults are always included
    )
}

// ❌ Wrong: Replace semantics — user list silently drops defaults
if len(cfg.SensitiveHeaders) == 0 {
    cfg.SensitiveHeaders = defaultSensitiveHeaders()
}
```

**Applies to:** Any config field whose defaults are security-critical
(redaction lists, required TLS settings, mandatory validations).
Ref: Design Spec Section 5.3 ("strict Redact List").

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

### Strip Context Headers Before Forwarding

Context headers (e.g., `X-Connect-Target-URL`, `X-Connect-Vendor-ID`) carry
internal metadata and must be stripped in the Director before forwarding.
The exact header list is defined in `context.HeaderSuffixes()` — strip only
those, not arbitrary prefix-matched headers. The trace header
(`Connect-Request-ID`) is preserved per Design Spec §8.3.

```go
// ✅ Correct: Strip exact headers from the canonical list
for _, suffix := range chaperoneCtx.HeaderSuffixes() {
    req.Header.Del(headerPrefix + suffix)
}

// ❌ Wrong: Prefix-match strips unrelated headers that happen to share the prefix
// ❌ Wrong: Strip before plugin call (breaks Slow Path plugins)
```

When iterating a map to delete entries (e.g., ranging over `http.Header`),
prefer a **two-pass collect-then-delete** pattern for clarity in
security-critical code:

```go
// ✅ Correct: Two-pass — unambiguous, no mutation-during-iteration questions
var toDelete []string
for header := range headers {
    if shouldStrip(header) {
        toDelete = append(toDelete, header)
    }
}
for _, h := range toDelete {
    headers.Del(h)
}

// ❌ Avoid in security paths: delete during range (safe per spec, but a code review footgun)
for header := range headers {
    if shouldStrip(header) {
        headers.Del(header) // readers pause to verify safety
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
