# Chaperone SDK

The **Chaperone SDK** provides the interfaces that Distributors implement to create custom plugins for the Chaperone egress proxy.

## Installation

```bash
go get github.com/cloudblue/chaperone/sdk
```

## Quick Start

Implement the `Plugin` interface:

```go
package myplugin

import (
    "context"
    "net/http"
    "time"

    "github.com/cloudblue/chaperone/sdk"
)

type MyPlugin struct{}

// GetCredentials injects authentication into outgoing requests
func (p *MyPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
    token := fetchTokenForVendor(tx.VendorID)
    return &sdk.Credential{
        Headers:   map[string]string{"Authorization": "Bearer " + token},
        ExpiresAt: time.Now().Add(55 * time.Minute),
    }, nil
}

// SignCSR handles certificate rotation (can return error if not needed)
func (p *MyPlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
    return nil, fmt.Errorf("certificate signing not implemented")
}

// ModifyResponse post-processes vendor responses
func (p *MyPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) error {
    resp.Header.Del("X-Internal-Debug-ID") // Strip internal headers
    return nil
}
```

## Testing Your Plugin

Use the compliance test kit to verify your implementation:

```go
package myplugin_test

import (
    "testing"
    
    "github.com/cloudblue/chaperone/sdk/compliance"
    "your/package/myplugin"
)

func TestMyPluginCompliance(t *testing.T) {
    plugin := &myplugin.MyPlugin{}
    compliance.VerifyContract(t, plugin)
}
```

## Interfaces

### CredentialProvider

Two strategies for credential injection:

| Strategy | Return Value | Caching | Use Case |
|----------|--------------|---------|----------|
| **Fast Path** | `*Credential, nil` | ✅ Cached by TTL | OAuth2 tokens, API keys |
| **Slow Path** | `nil, nil` | ❌ Not cached | HMAC signing, dynamic auth |

### CertificateSigner

Called by the proxy core when certificate rotation is needed. Forward the CSR to your CA (Connect API, Vault, etc.).

### ResponseModifier

Post-process responses before returning to upstream. Use for stripping PII, normalizing errors, or logging.

## Building Your Binary

Once you've implemented the `Plugin` interface, build your custom binary using `chaperone.Run()`:

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
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    if err := chaperone.Run(ctx, myplugin.New(),
        chaperone.WithConfigPath("/etc/chaperone.yaml"),
        chaperone.WithVersion("1.0.0"),
    ); err != nil {
        os.Exit(1)
    }
}
```

See the [Chaperone README](../README.md#building-custom-binaries-distributor-workflow) for all available options.

## Module Versioning

This SDK is versioned **independently** from the Chaperone core. You can safely upgrade the proxy core without modifying your plugin code, as long as the SDK major version (`v1`) remains stable.

```go
require (
    github.com/cloudblue/chaperone/sdk v1.0.0  // SDK - stable interface
    github.com/cloudblue/chaperone v1.5.0      // Core - can upgrade freely
)
```
