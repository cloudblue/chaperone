# Plugin Development Guide

How to build a custom Chaperone plugin that injects credentials into
outgoing API requests. By the end of this guide, you'll have a working
plugin that injects a custom header into every proxied request.

**Time:** ~30 minutes

**What you'll learn:**
- How to create a plugin project from scratch
- How to implement the [`Plugin`](../reference/sdk.md#plugin-composite-interface) interface
- How to build, run, and verify credential injection end-to-end
- How to test your plugin
- Common credential patterns for real-world use

> **SDK Reference:** For complete interface definitions, type fields, and
> method signatures, see the [SDK Reference](../reference/sdk.md).

## Prerequisites

| Requirement | Version | Purpose |
|-------------|---------|---------|
| **Go** | 1.25+ | Building the plugin binary ([install Go](https://go.dev/doc/install)) |
| **curl** | any | Sending test requests |

> **Recommended:** Complete the [Getting Started](../getting-started.md)
> tutorial first. It introduces the proxy, configuration, allow-lists,
> and request flow — all concepts used here.

Chaperone compiles your plugin directly into the proxy binary (static
recompilation). You implement the
[`sdk.Plugin`](../reference/sdk.md#plugin-composite-interface) interface,
pass it to [`chaperone.Run()`](../reference/sdk.md#run), and the proxy
handles TLS, routing, caching, logging, and response sanitization. See the
[Design Specification](../explanation/DESIGN-SPECIFICATION.md) for the
rationale behind this approach.

## Build your first plugin

This section walks you through creating a complete plugin project from
scratch. You'll create your own Go module with a plugin that injects a
custom header, build a proxy binary, and see it appear in a real HTTP
response.

This is the **"Own Repo" method** — the recommended workflow for
production deployments. For a simpler alternative that doesn't require
a separate repository, see [Fork/Extend](#alternative-forkextend) below.

### Step 1: Create the Project

Create a new directory for your proxy project:

```bash
cd ~/projects
mkdir -p my-proxy/plugins
cd my-proxy
```

### Step 2: Initialize the Go Module

Initialize your Go module and add the Chaperone dependencies:

```bash
go mod init github.com/acme/my-proxy
go get github.com/cloudblue/chaperone@latest
go get github.com/cloudblue/chaperone/sdk@latest
```

This creates a `go.mod` like:

```go
module github.com/acme/my-proxy

go 1.25

require (
    github.com/cloudblue/chaperone     v0.1.0
    github.com/cloudblue/chaperone/sdk v0.1.0
)
```

The SDK and Core are versioned independently (see
[Module Versioning](../reference/sdk.md#module-versioning)). You can
upgrade the Core (bug fixes, performance) without changing your plugin
code, as long as the SDK major version remains the same.

> **Contributing to Chaperone?** If you need to test against a local
> checkout of the Chaperone source (e.g., for development), use a
> [Go workspace](https://go.dev/doc/tutorial/workspaces) instead of
> `replace` directives:
> ```bash
> go work init . /path/to/chaperone /path/to/chaperone/sdk
> ```

### Step 3: Write Your Plugin

Create `plugins/myplugin.go`. The central method is `GetCredentials`,
which supports two strategies:

| Strategy | Return Value | When to Use |
|----------|-------------|-------------|
| **Fast Path** | `*Credential` with headers + TTL | Static tokens, API keys, Bearer tokens — cached by the proxy |
| **Slow Path** | `nil, nil` (mutate `req` directly) | HMAC body signing, request-dependent auth — runs every request |

This guide uses the Fast Path. See [HMAC Body Signing](#hmac-body-signing-slow-path)
for a Slow Path example.

```go
package plugins

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/cloudblue/chaperone/sdk"
)

// MyPlugin implements sdk.Plugin with a simple credential injection strategy.
type MyPlugin struct {
    // In production, you'd store a Vault client, database pool,
    // token cache, or other credential source here.
}

// New creates a new MyPlugin instance.
func New() *MyPlugin {
    return &MyPlugin{}
}

// GetCredentials is called by the proxy for every incoming request.
// It returns the credentials to inject into the outgoing request.
//
// This example uses the Fast Path strategy: return a *Credential with
// the headers to inject and a cache TTL. The proxy caches this result
// and won't call your plugin again until ExpiresAt passes.
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    tag, err := p.fetchTag(ctx, tx.VendorID)
    if err != nil {
        return nil, fmt.Errorf("fetching tag for vendor %s: %w", tx.VendorID, err)
    }

    // Hello World: inject a non-sensitive demo header.
    // In production: "Authorization": "Bearer " + token
    // See "Common Credential Patterns" for real-world examples.
    return &sdk.Credential{
        Headers:   map[string]string{"X-Injected-By": tag},
        ExpiresAt: time.Now().Add(55 * time.Minute),
    }, nil
}

// SignCSR handles certificate rotation. Return an error if you manage
// certificates externally (most deployments).
func (p *MyPlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
    return nil, fmt.Errorf("certificate signing not implemented")
}

// ModifyResponse lets you post-process vendor responses. Return nil, nil
// to use the default behavior (Core applies error normalization).
func (p *MyPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
    return nil, nil
}

// fetchTag returns the value to inject for a given vendor.
// Replace this with your real credential source (Vault, database, API, etc.).
func (p *MyPlugin) fetchTag(_ context.Context, vendorID string) (string, error) {
    // Hello World: return a non-sensitive demo value.
    // In production, this would return a real token, API key, etc.
    return "chaperone-hello-world", nil
}
```

The plugin implements three interfaces (see the
[SDK Reference](../reference/sdk.md#plugin-composite-interface) for
complete documentation):

| Method | Purpose | Our implementation |
|--------|---------|-------------------|
| [`GetCredentials`](../reference/sdk.md#credentialprovider) | Inject headers into outgoing requests | Injects a demo header (Fast Path) |
| [`SignCSR`](../reference/sdk.md#certificatesigner) | Sign certificate rotation requests | Stub — certificates managed externally |
| [`ModifyResponse`](../reference/sdk.md#responsemodifier) | Post-process vendor responses before returning to platform | Default behavior (error normalization) |

### Step 4: Write main.go

Create `main.go` in the project root. This is the entry point that wires
your plugin into the Chaperone proxy:

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/cloudblue/chaperone"
    myplugin "github.com/acme/my-proxy/plugins"
)

func main() {
    // Set up signal handling for graceful shutdown (Ctrl+C or SIGTERM).
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    // Start the proxy with your plugin.
    // Chaperone loads config from ./config.yaml by default.
    if err := chaperone.Run(ctx, myplugin.New(),
        chaperone.WithVersion("0.1.0"),
    ); err != nil {
        os.Exit(1)
    }
}
```

> **Where does the config come from?** If you don't pass
> [`WithConfigPath`](../reference/sdk.md#withconfigpath), Chaperone looks
> for a config file in this order: (1) `CHAPERONE_CONFIG` environment
> variable, (2) `./config.yaml` in the current directory. We'll create
> that file next.

### Step 5: Add a Configuration File

Create `config.yaml` in the project root. This is the same configuration
concept from the [Getting Started](../getting-started.md) tutorial — it
tells Chaperone which hosts your proxy is allowed to reach:

```yaml
# config.yaml — Tutorial configuration for plugin development.
# ⚠️  Not for production use. Enable TLS and restrict the allow-list.

server:
  addr: ":8443"
  admin_addr: ":9090"
  tls:
    enabled: false            # No mTLS for local development

upstream:
  allow_list:
    "httpbin.org":
      - "/**"                 # Allow all paths on httpbin.org
```

> **Why do we need an allow-list?** Chaperone is a security-first proxy.
> It refuses to forward requests to hosts not in the allow-list, preventing
> the proxy from being used to reach arbitrary destinations. See the
> [Configuration Reference](../reference/configuration.md) for all options.

### Step 6: Resolve Dependencies and Build

Go modules require a dependency resolution step before building. Run
`go mod tidy` to download the Chaperone modules and create the `go.sum`
lock file:

```bash
go mod tidy
```

This will add indirect dependencies to your `go.mod` and create a
`go.sum` file — both are normal and should be committed to version control.

Now build your binary:

```bash
go build -o my-proxy .
```

Your project should now look like:

```
my-proxy/
├── go.mod
├── go.sum              ← Created by go mod tidy
├── main.go
├── my-proxy            ← Your compiled binary
├── config.yaml
└── plugins/
    └── myplugin.go
```

### Step 7: Run and Verify

Start your proxy:

```bash
./my-proxy
```

You should see JSON-formatted log messages indicating the proxy is ready
(look for `"admin server started"` and `"server listening"`). If you
completed the [Getting Started](../getting-started.md) tutorial, this
output will look familiar.

Now **open a new terminal** and let's verify your plugin works with a
before/after comparison. First, send a request **directly** to httpbin
(no proxy) to see the normal headers:

```bash
curl -s https://httpbin.org/headers
```

You'll see the standard headers that curl sends — `Accept`, `Host`,
`User-Agent`, and nothing else.

Now send the same request **through your proxy**:

```bash
curl -s http://localhost:8443/proxy \
  -H "X-Connect-Target-URL: https://httpbin.org/headers" \
  -H "X-Connect-Vendor-ID: test-vendor"
```

Compare the two responses. The proxied response has an extra header that
wasn't in the direct request:

```
"X-Injected-By": "chaperone-hello-world"
```

**That's your plugin working.** Chaperone called your `GetCredentials`
method, got the value you returned, and injected it as a header into the
outgoing request. The `httpbin.org/headers` endpoint echoes back all
request headers it received, so you can see exactly what arrived.

> **In production**, your plugin would inject real authentication
> credentials — for example, `Authorization: Bearer <token>` or
> `X-API-Key: <secret>`. We use a non-sensitive demo header here because
> httpbin echoes everything back, and we don't want the tutorial to look
> like credential leaking — the exact thing Chaperone is designed to
> prevent! See [Common Credential Patterns](#common-credential-patterns)
> for real-world examples.

### Step 8: Clean Up

Press `Ctrl+C` in the terminal running your proxy to stop it.

---

### Going Further

Now that you have a working plugin, here's how to add more capabilities.

#### Adding Enrollment Support

For production mTLS deployments, your binary can generate Certificate
Signing Requests (CSRs). Add enrollment support with a simple subcommand
check in `main.go`:

```go
func main() {
    if len(os.Args) > 1 && os.Args[1] == "enroll" {
        if err := runEnroll(os.Args[2:]); err != nil {
            fmt.Fprintf(os.Stderr, "enrollment failed: %v\n", err)
            os.Exit(1)
        }
        return
    }

    // Normal proxy startup...
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := chaperone.Run(ctx, myplugin.New(),
        chaperone.WithVersion("0.1.0"),
    ); err != nil {
        os.Exit(1)
    }
}

func runEnroll(args []string) error {
    fs := flag.NewFlagSet("enroll", flag.ExitOnError)
    domains := fs.String("domains", "", "Comma-separated DNS names and IPs")
    outDir := fs.String("out", "./certs", "Output directory")
    fs.Parse(args)

    result, err := chaperone.Enroll(context.Background(), chaperone.EnrollConfig{
        Domains:   *domains,
        OutputDir: *outDir,
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
./my-proxy enroll -domains proxy.example.com
```

See [Certificate Management](certificate-management.md) for the full
enrollment workflow, including submitting the CSR to a CA.

#### Available Options

See [Option Functions](../reference/sdk.md#option-functions) in the SDK
Reference for all options you can pass to
[`chaperone.Run()`](../reference/sdk.md#run):
[`WithConfigPath`](../reference/sdk.md#withconfigpath),
[`WithVersion`](../reference/sdk.md#withversion),
[`WithBuildInfo`](../reference/sdk.md#withbuildinfo),
[`WithLogOutput`](../reference/sdk.md#withlogoutput).

#### Docker Deployment

See the [Deployment Guide](deployment.md) for complete Docker deployment
instructions including Dockerfile templates, production hardening, and
Kubernetes probe configuration.

---

## Alternative: Fork/Extend

Instead of creating a separate repository, you can add your plugin
directly to the Chaperone source tree. This is simpler to set up but
couples your plugin to the Chaperone repository — upgrading means merging
upstream changes.

**Best for:** Quick proof-of-concept, simple deployments with infrequent
upgrades, or when you don't need independent version control.

### Steps

1. **Clone the repository** (if you haven't already):

```bash
git clone https://github.com/cloudblue/chaperone.git
cd chaperone
```

2. **Create your plugin** under `plugins/`:

```bash
mkdir -p plugins/myplugin
```

Create `plugins/myplugin/myplugin.go` with your
[`sdk.Plugin`](../reference/sdk.md#plugin-composite-interface)
implementation — the same plugin code from
[Step 3](#step-3-write-your-plugin) above works here. Change only the
package declaration and import path:

```go
package myplugin  // ← was "plugins" in the Own Repo method
```

3. **Modify `cmd/chaperone/main.go`** to use your plugin:

```go
import (
    myplugin "github.com/cloudblue/chaperone/plugins/myplugin"
)

// In main(), replace the existing plugin with yours:
plugin := myplugin.New()
```

4. **Build and run** using the same steps as the tutorial:

```bash
make build
./bin/chaperone
```

Then verify with the same curl command from
[Step 7](#step-7-run-and-verify) — you should see
`"X-Injected-By": "chaperone-hello-world"` in the httpbin response.

### Tradeoffs

| Aspect | Fork/Extend | Own Repo |
|--------|-------------|----------|
| Setup complexity | Low (clone + edit) | Medium (new module) |
| Upgrade path | Git merge (may conflict) | Update `require` version |
| Version independence | Coupled | Independent |
| CI/CD | Shared pipeline | Your own pipeline |
| **Recommended for production** | No | **Yes** |

---

## Testing Your Plugin

### Unit Tests

Go tests live alongside the code they test. Create
`plugins/myplugin_test.go` in the same directory as your plugin:

```go
package plugins_test

import (
    "context"
    "net/http"
    "testing"
    "time"

    "github.com/acme/my-proxy/plugins"
    "github.com/cloudblue/chaperone/sdk"
)

func TestGetCredentials_ReturnsCredential(t *testing.T) {
    p := plugins.New()

    tx := sdk.TransactionContext{
        VendorID:  "vendor-123",
        ProductID: "product-456",
        TargetURL: "https://api.vendor.com/v1/orders",
    }
    req, _ := http.NewRequest("GET", tx.TargetURL, nil)

    cred, err := p.GetCredentials(context.Background(), tx, req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cred == nil {
        t.Fatal("expected credential, got nil")
    }

    injected := cred.Headers["X-Injected-By"]
    if injected == "" {
        t.Error("expected X-Injected-By header, got empty string")
    }
    if cred.ExpiresAt.Before(time.Now()) {
        t.Error("credential already expired")
    }
}
```

Run from the project root:

```bash
go test ./plugins/...
```

You should see output like:

```
ok      github.com/acme/my-proxy/plugins    0.003s
```

> **Test file naming:** Go requires test files to end with `_test.go`.
> The test package can be the same (`plugins`) for white-box testing, or
> `plugins_test` (as above) for black-box testing that only uses exported
> functions.

### Compliance Suite

The SDK includes a compliance test suite that validates your plugin
implements all required interfaces and handles edge cases correctly.
Add a compliance test in `plugins/compliance_test.go`:

```go
package plugins_test

import (
    "testing"

    "github.com/acme/my-proxy/plugins"
    "github.com/cloudblue/chaperone/sdk/compliance"
)

func TestPluginCompliance(t *testing.T) {
    plugin := plugins.New()
    compliance.VerifyContract(t, plugin)
}
```

Run it the same way:

```bash
go test ./plugins/...
```

The compliance suite verifies:

- All three interfaces are implemented ([`CredentialProvider`](../reference/sdk.md#credentialprovider), [`CertificateSigner`](../reference/sdk.md#certificatesigner), [`ResponseModifier`](../reference/sdk.md#responsemodifier))
- Plugin handles nil/empty inputs without panicking
- Returned credentials have valid expiry times (non-zero, in the future)
- Context cancellation is handled gracefully

### Integration Tests

For tests that verify end-to-end behavior with a mock vendor server,
create `plugins/integration_test.go`:

```go
package plugins_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/acme/my-proxy/plugins"
    "github.com/cloudblue/chaperone/sdk"
)

func TestPlugin_InjectsHeader(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Start a mock vendor server that verifies the header was injected.
    vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        injected := r.Header.Get("X-Injected-By")
        if injected == "" {
            t.Error("missing X-Injected-By header in proxied request")
        }
        w.WriteHeader(http.StatusOK)
    }))
    defer vendor.Close()

    // Call your plugin with a test context.
    p := plugins.New()
    tx := sdk.TransactionContext{
        VendorID:  "test-vendor",
        TargetURL: vendor.URL,
    }
    req, _ := http.NewRequest("GET", vendor.URL, nil)

    cred, err := p.GetCredentials(context.Background(), tx, req)
    if err != nil {
        t.Fatalf("GetCredentials failed: %v", err)
    }

    // Apply the credentials like the proxy would.
    for k, v := range cred.Headers {
        req.Header.Set(k, v)
    }

    // Send the request to the mock vendor.
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("request to mock vendor failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("expected 200, got %d", resp.StatusCode)
    }
}
```

Run integration tests:

```bash
go test ./plugins/... -v
```

Skip them in CI fast-feedback loops with `go test -short ./plugins/...`.

---

## Common Credential Patterns

> **Shortcut:** For OAuth2 client credentials, refresh token grants, and
> Microsoft Secure Application Model, the
> [contrib building blocks](../reference/contrib-plugins.md) provide
> production-ready implementations with token caching, singleflight
> deduplication, and expiry margin handling. Use the
> [request multiplexer](../reference/contrib-plugins.md#mux) to route
> different vendors to different building blocks without writing auth
> logic from scratch. The [Microsoft SAM with Mux tutorial](../tutorials/microsoft-sam-mux.md) walks through the full setup end-to-end.
>
> **Initial token setup:** Refresh token building blocks require a pre-seeded
> token store. See [Onboarding Refresh Tokens](onboarding-refresh-tokens.md)
> for how to bootstrap the first token using the `chaperone-onboard` CLI.

Now that you have a working plugin, here are patterns for real-world
credential strategies. Each pattern replaces the `fetchTag` method
and `GetCredentials` return logic in your plugin.

### Bearer Token (Fast Path)

**When to use:** The vendor API expects an `Authorization: Bearer <token>`
header. The token comes from your credential store and can be cached.

**How it works:** Look up the token by vendor ID, return it in a
[`Credential`](../reference/sdk.md#credential) with a cache TTL set
slightly before the token's actual expiry.

```go
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    token, err := p.tokenStore.Get(ctx, tx.VendorID)
    if err != nil {
        return nil, fmt.Errorf("fetching bearer token: %w", err)
    }

    return &sdk.Credential{
        Headers:   map[string]string{"Authorization": "Bearer " + token},
        ExpiresAt: time.Now().Add(55 * time.Minute),
    }, nil
}
```

**Injected header:** `Authorization: Bearer <token>`

### API Key (Fast Path)

**When to use:** The vendor expects a static API key in a custom header
(e.g., `X-API-Key`). Common for simple REST APIs.

**How it works:** Same as Bearer Token, but the header name and format
differ. API keys rarely expire, so use a long cache TTL.

```go
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    key, err := p.keyStore.Get(ctx, tx.VendorID)
    if err != nil {
        return nil, fmt.Errorf("fetching API key: %w", err)
    }

    return &sdk.Credential{
        Headers:   map[string]string{"X-API-Key": key},
        ExpiresAt: time.Now().Add(24 * time.Hour),
    }, nil
}
```

**Injected header:** `X-API-Key: <key>`

### OAuth2 Token Refresh (Fast Path with Short TTL)

**When to use:** The vendor uses OAuth2 and tokens expire frequently.
Your plugin needs to refresh the token when it expires.

**How it works:** Call your OAuth2 provider, get the token and its expiry
time, then set the cache TTL to 5 minutes *before* the real expiry — this
ensures the proxy refreshes the token before it actually expires.

> `p.oauth` is your OAuth2 client (e.g., using
> [`golang.org/x/oauth2/clientcredentials`](https://pkg.go.dev/golang.org/x/oauth2/clientcredentials)).
> The Chaperone-specific part is the cache TTL strategy below.

```go
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    // p.oauth wraps your OAuth2 client_credentials flow.
    token, expiresAt, err := p.oauth.GetOrRefreshToken(ctx, tx.VendorID)
    if err != nil {
        return nil, fmt.Errorf("OAuth2 token refresh: %w", err)
    }

    // Refresh 5 minutes before actual expiry to avoid using a stale token.
    cacheExpiry := expiresAt.Add(-5 * time.Minute)
    if cacheExpiry.Before(time.Now()) {
        cacheExpiry = time.Now().Add(1 * time.Minute)
    }

    return &sdk.Credential{
        Headers:   map[string]string{"Authorization": "Bearer " + token},
        ExpiresAt: cacheExpiry,
    }, nil
}
```

**Injected header:** `Authorization: Bearer <oauth-token>`

### HMAC Body Signing (Slow Path)

**When to use:** The vendor requires a cryptographic signature computed
over the request body (e.g., webhook verification, AWS Signature V4).
Since the signature depends on the body content, it can't be cached.

**How it works:** Read the request body, compute an HMAC signature,
set signature headers directly on the request, and return `nil, nil`
(Slow Path). The proxy detects the added headers and ensures they are
redacted from logs and stripped from responses.

```go
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    // Read and restore the body (the proxy still needs it).
    body, err := io.ReadAll(req.Body)
    if err != nil {
        return nil, fmt.Errorf("reading request body: %w", err)
    }
    req.Body = io.NopCloser(bytes.NewReader(body))

    // Compute HMAC-SHA256 signature.
    mac := hmac.New(sha256.New, p.secretKey)
    mac.Write(body)
    signature := hex.EncodeToString(mac.Sum(nil))

    // Slow Path: mutate the request directly.
    req.Header.Set("X-Signature", signature)
    req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

    return nil, nil // Signals Slow Path to the proxy
}
```

**Injected headers:** `X-Signature: <hmac>`, `X-Timestamp: <time>`

### HashiCorp Vault Lookup (Fast Path)

**When to use:** Credentials are stored in HashiCorp Vault (or a similar
secrets manager). Your plugin fetches them over the network.

**How it works:** Build an HTTP request to Vault's API *using the context*
from the proxy (so it respects timeouts and cancellation), extract the
secret, and return it as a cached credential.

```go
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    // Use ctx so the request is cancelled if the proxy times out.
    vaultReq, err := http.NewRequestWithContext(ctx, "GET",
        fmt.Sprintf("%s/v1/secret/data/vendors/%s", p.vaultAddr, tx.VendorID), nil)
    if err != nil {
        return nil, fmt.Errorf("creating vault request: %w", err)
    }
    vaultReq.Header.Set("X-Vault-Token", p.vaultToken)

    resp, err := p.httpClient.Do(vaultReq)
    if err != nil {
        return nil, fmt.Errorf("vault request failed: %w", err)
    }
    defer resp.Body.Close()

    // Parse Vault response and extract the vendor's token.
    token := parseVaultResponse(resp)

    return &sdk.Credential{
        Headers:   map[string]string{"Authorization": "Bearer " + token},
        ExpiresAt: time.Now().Add(1 * time.Hour),
    }, nil
}
```

**Injected header:** `Authorization: Bearer <vault-secret>`

> **Context tip:** Always use `http.NewRequestWithContext(ctx, ...)` for
> network calls in your plugin. The `ctx` parameter from the proxy has a
> timeout and is cancelled if the client disconnects. This prevents your
> plugin from leaking goroutines or holding connections to slow backends.

---

## Reference Plugin Walkthrough

The Reference Plugin (`plugins/reference/reference.go`) in the Chaperone
source demonstrates a complete, production-ready implementation:

- **Credential source:** JSON file with vendor → auth mappings
- **Strategy:** Fast Path (returns [`*Credential`](../reference/sdk.md#credential) with TTL)
- **Auth types:** `bearer`, `api_key`, `basic`
- **Thread safety:** In-memory map with `sync.RWMutex` protection
- **Certificate signing:** Stub (returns error)
- **Response modification:** Default behavior (`nil, nil`)

**JSON credentials file format:**

```json
{
  "vendors": {
    "vendor-123": {
      "auth_type": "bearer",
      "token": "your-api-token",
      "ttl_minutes": 60
    },
    "vendor-456": {
      "auth_type": "api_key",
      "header_name": "X-API-Key",
      "token": "your-api-key",
      "ttl_minutes": 1440
    }
  }
}
```

Study this plugin as a starting template, especially the
`sync.RWMutex` pattern for thread-safe credential access.

---

## Next Steps

- [Contrib Plugins Reference](../reference/contrib-plugins.md) — Pre-built OAuth2, Microsoft SAM, and request multiplexer
- [SDK Reference](../reference/sdk.md) — Complete interface and type definitions
- [Deployment Guide](deployment.md) — Deploy your custom binary with Docker
- [Certificate Management](certificate-management.md) — Set up mTLS for production
- [Configuration Reference](../reference/configuration.md) — All config options
- [Troubleshooting](troubleshooting.md) — Common issues and solutions