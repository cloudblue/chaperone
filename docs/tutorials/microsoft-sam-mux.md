# Set up Microsoft SAM with the request multiplexer

Build a Chaperone proxy that authenticates requests to Microsoft APIs using the Secure Application Model (SAM) and the contrib request multiplexer. By the end, you'll have a working binary that exchanges refresh tokens for access tokens and injects `Authorization: Bearer` headers into outgoing requests.

**Time:** ~20 minutes

## Prerequisites

| Requirement | Version | Purpose |
|-------------|---------|---------|
| **Go** | 1.25+ | Building the proxy binary ([install Go](https://go.dev/doc/install)) |
| **Chaperone source** | — | SDK, Core, and Contrib modules (cloned in [Getting Started](../getting-started.md)) |
| **curl** | any | Sending test requests |
| **Microsoft Entra ID app registration** | — | Client ID, client secret, and an account with admin consent permissions ([quickstart](https://learn.microsoft.com/en-us/entra/identity-platform/quickstart-register-app)) |
| **Target resource URI** | — | e.g., `https://graph.microsoft.com` |
| **Tenant ID** | — | e.g., `contoso.onmicrosoft.com` |

> **New to Chaperone?** Complete the [Getting Started](../getting-started.md) tutorial first. It introduces the proxy, configuration, allow-lists, and request flow.

## Step 1: Create the project

Create a new directory next to the Chaperone source:

```bash
cd ~/projects
mkdir sam-proxy
cd sam-proxy
go mod init github.com/acme/sam-proxy
```

## Step 2: Write `main.go`

Create `main.go` first — `go mod tidy` needs it to resolve imports.

The file wires the Microsoft refresh token source into the Mux and passes it to `chaperone.Run`:

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/cloudblue/chaperone"
    "github.com/cloudblue/chaperone/plugins/contrib"
    "github.com/cloudblue/chaperone/plugins/contrib/microsoft"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    store := microsoft.NewFileStore("tokens") // base directory for per-tenant token files
    source := microsoft.NewRefreshTokenSource(microsoft.Config{
        ClientID:     os.Getenv("AZURE_CLIENT_ID"),
        ClientSecret: os.Getenv("AZURE_CLIENT_SECRET"),
        Store:        store,
    })

    mux := contrib.NewMux()
    mux.Handle(contrib.Route{
        TargetURL: "graph.microsoft.com/**",
    }, source)

    if err := chaperone.Run(ctx, mux); err != nil {
        os.Exit(1)
    }
}
```

The Mux routes requests by matching the target URL. Any request to `graph.microsoft.com` goes to the Microsoft SAM source, which extracts `TenantID` and `Resource` from the transaction context data map and returns a bearer token.

Now add the `replace` directives (needed until all modules are published) and resolve dependencies:

```bash
cat >> go.mod << 'EOF'

replace (
    github.com/cloudblue/chaperone                => ../chaperone
    github.com/cloudblue/chaperone/sdk            => ../chaperone/sdk
    github.com/cloudblue/chaperone/plugins/contrib => ../chaperone/plugins/contrib
)
EOF

go mod tidy
```

The command creates `go.sum` with no errors.

Build the binary:

```bash
go build -o sam-proxy .
```

You should see `sam-proxy` in the current directory.

## Step 3: Add `config.yaml`

Create `config.yaml` with `graph.microsoft.com` in the allow-list:

```yaml
server:
  addr: ":8443"
  admin_addr: ":9090"
  tls:
    enabled: false

upstream:
  allow_list:
    "graph.microsoft.com":
      - "/**"
```

## Step 4: Bootstrap the refresh token

Build the onboarding CLI from the Chaperone source:

```bash
(cd ~/projects/chaperone && make build-onboard)
```

Run it with your Entra ID credentials. The tool opens your browser for admin consent and prints the refresh token to stdout:

```bash
export CHAPERONE_ONBOARD_CLIENT_SECRET='your-app-secret'

~/projects/chaperone/bin/chaperone-onboard microsoft \
  -tenant contoso.onmicrosoft.com \
  -client-id 12345678-abcd-1234-abcd-1234567890ab \
  -resource https://graph.microsoft.com \
  > refresh-token.txt
```

Sign in as an admin and grant consent. The browser shows a completion page, and `refresh-token.txt` appears with the token.

> For sovereign clouds (Azure Government, Azure China), add `-endpoint https://login.microsoftonline.us`. See [Onboarding Refresh Tokens](../guides/onboarding-refresh-tokens.md) for troubleshooting.

## Step 5: Seed the token store

The `microsoft.FileStore` expects tokens in a `{baseDir}/{tenantID}/{sanitizedResource}` directory tree. Create the directory and move the token file:

```bash
mkdir -p tokens/contoso.onmicrosoft.com
mv refresh-token.txt tokens/contoso.onmicrosoft.com/graph.microsoft.com
```

Verify:

```bash
ls tokens/contoso.onmicrosoft.com/
```

You should see `graph.microsoft.com`. The resource filename is derived by stripping the URL scheme and replacing any character outside `[a-zA-Z0-9._-]` with an underscore — `https://graph.microsoft.com` becomes `graph.microsoft.com` (no replacements needed in this case, but a resource like `https://api.example.com/v1` would become `api.example.com_v1`). See [FileStore reference](../reference/contrib-plugins.md#microsoft-filestore) for the full sanitization rules.

## Step 6: Run the proxy

Set your Entra ID credentials and start the proxy:

```bash
export AZURE_CLIENT_ID='12345678-abcd-1234-abcd-1234567890ab'
export AZURE_CLIENT_SECRET='your-app-secret'

./sam-proxy
```

Watch the JSON logs for `"server listening"` — the proxy is ready.

## Step 7: Send a request

Open a new terminal and send a request through the proxy. The `X-Connect-Context-Data` header carries the tenant and resource as a Base64-encoded JSON object:

```bash
curl -s http://localhost:8443/proxy \
  -H "X-Connect-Target-URL: https://graph.microsoft.com/v1.0/me" \
  -H "X-Connect-Vendor-ID: microsoft-partner" \
  -H "X-Connect-Context-Data: $(echo -n '{"TenantID":"contoso.onmicrosoft.com","Resource":"https://graph.microsoft.com"}' | base64 | tr -d '\n')"
```

Two outcomes confirm the setup works:

- **A Microsoft Graph response** (e.g., user profile JSON) — the bearer token was injected and accepted.
- **A `401` or `403` from Microsoft** — the proxy, mux, and token exchange all worked; the token lacks the required permissions for this specific API call. Adjust your app registration's API permissions and try again.

When you are done testing, press `Ctrl+C` to stop the proxy.

## What you built

Your project has this structure:

```
sam-proxy/
├── go.mod
├── go.sum
├── main.go
├── sam-proxy
├── config.yaml
└── tokens/
    └── contoso.onmicrosoft.com/
        └── graph.microsoft.com
```

The proxy handles the full auth lifecycle:

- Reads the seeded refresh token from the file store.
- Exchanges it for an access token at Microsoft's token endpoint.
- Injects `Authorization: Bearer <token>` into the outgoing request.
- Caches the access token until it expires.
- Persists any rotated refresh tokens back to disk.

## Going further

- **Add more tenants or resources** — Run `chaperone-onboard microsoft` for each tenant+resource pair and place the token in the directory tree. The `RefreshTokenSource` manages an LRU pool of token sessions automatically.
- **Route multiple vendors** — Register additional `mux.Handle` calls with different routes and providers. See the [Mux reference](../reference/contrib-plugins.md#mux) for route specificity and glob patterns.
- **Onboarding details** — [Onboarding Refresh Tokens](../guides/onboarding-refresh-tokens.md) covers sovereign clouds, troubleshooting, and alternative storage backends.
- **Full API surface** — [Contrib Plugins Reference](../reference/contrib-plugins.md) documents all `Config` fields, sentinel errors, and the Vault-backed `TokenStore` skeleton.
- **Custom plugins alongside the Mux** — [Plugin Development Guide](../guides/plugin-development.md) covers writing your own `CredentialProvider`, testing with the compliance suite, and HMAC body signing.
- **Production deployment** — [Deployment Guide](../guides/deployment.md) for Docker, hardening, and Kubernetes probes. [Certificate Management](../guides/certificate-management.md) for mTLS setup.
