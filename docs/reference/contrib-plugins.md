# Contrib plugins reference

Reusable auth flow building blocks and a request multiplexer for the Chaperone egress proxy.

The contrib module is a separate Go module (`github.com/cloudblue/chaperone/plugins/contrib`) that depends on the [SDK](sdk.md) and provides two layers:

- **Building blocks** implement [`CredentialProvider`][cp] and handle a single auth flow (OAuth2 client credentials, refresh token grant, Microsoft Secure Application Model).
- **Mux** implements the full [`Plugin`][plugin] interface and routes requests to the right building block by vendor, environment, or target URL.

Sub-packages:

| Package | Import path | Purpose |
|---------|-------------|---------|
| `contrib` | `github.com/cloudblue/chaperone/plugins/contrib` | Mux, Route, errors, adapter |
| `contrib/oauth` | `github.com/cloudblue/chaperone/plugins/contrib/oauth` | Generic OAuth2 grants |
| `contrib/microsoft` | `github.com/cloudblue/chaperone/plugins/contrib/microsoft` | Microsoft Secure Application Model |

**Go standard library types** used in signatures link to official documentation: [`context.Context`][ctx], [`*http.Request`][req], [`*http.Response`][resp], [`time.Time`][time], [`time.Duration`][dur], [`*slog.Logger`][slog].

[ctx]: https://pkg.go.dev/context#Context
[req]: https://pkg.go.dev/net/http#Request
[client]: https://pkg.go.dev/net/http#Client
[resp]: https://pkg.go.dev/net/http#Response
[time]: https://pkg.go.dev/time#Time
[dur]: https://pkg.go.dev/time#Duration
[slog]: https://pkg.go.dev/log/slog#Logger
[cp]: sdk.md#credentialprovider
[plugin]: sdk.md#plugin-composite-interface
[tx]: sdk.md#transactioncontext
[cred]: sdk.md#credential
[cs]: sdk.md#certificatesigner
[rm]: sdk.md#responsemodifier

---

## Mux

```go
type Mux struct{ /* unexported */ }
```

A request multiplexer that dispatches to the most specific matching [`CredentialProvider`][cp] based on transaction context fields. `Mux` implements [`Plugin`][plugin] and can be passed directly to `chaperone.Run()`.

Safe for concurrent use after route registration is complete. Register all routes before serving traffic.

### `NewMux`

```go
func NewMux(opts ...MuxOption) *Mux
```

Creates a new multiplexer. Pass zero or more [`MuxOption`](#muxoption) values to configure behavior.

### `MuxOption`

```go
type MuxOption func(*Mux)
```

#### `WithLogger`

```go
func WithLogger(l *slog.Logger) MuxOption
```

Sets the logger for mux warnings (e.g., tie-breaking). Defaults to [`slog.Default()`][slog].

### `Handle`

```go
func (m *Mux) Handle(route Route, provider sdk.CredentialProvider)
```

Registers a route that dispatches matching requests to `provider`. Routes are evaluated by [specificity](#specificity) at dispatch time. Registration order breaks ties.

### `Default`

```go
func (m *Mux) Default(provider sdk.CredentialProvider)
```

Sets a fallback provider used when no registered route matches.

### `SetSigner`

```go
func (m *Mux) SetSigner(signer sdk.CertificateSigner)
```

Configures the [`CertificateSigner`][cs] delegate. Without a signer, `SignCSR` returns an error.

### `SetResponseModifier`

```go
func (m *Mux) SetResponseModifier(modifier sdk.ResponseModifier)
```

Configures the [`ResponseModifier`][rm] delegate. Without a modifier, `ModifyResponse` returns a nil action and nil error (no-op).

### `GetCredentials`

```go
func (m *Mux) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error)
```

Dispatches the request to the best matching route's provider:

1. All registered routes are tested against `tx`.
2. The match with the highest [specificity](#specificity) wins.
3. If multiple matches share the highest specificity, the first registered route wins and a warning is logged.
4. If no route matches, the default provider is used.
5. If no route matches and no default is configured, returns [`ErrNoRouteMatch`](#sentinel-errors).

### `SignCSR`

```go
func (m *Mux) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error)
```

Delegates to the configured signer. Returns an error if no signer has been set.

### `ModifyResponse`

```go
func (m *Mux) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error)
```

Delegates to the configured modifier. Returns a nil action and nil error if no modifier has been set.

---

## Route

```go
type Route struct {
    VendorID      string
    TargetURL     string
    EnvironmentID string
}
```

Matching criteria for dispatching requests. Each non-empty field must match the corresponding [`TransactionContext`][tx] field. Empty fields act as wildcards. All fields support [glob patterns](#glob-patterns).

| Field | Matches against | Example pattern |
|-------|----------------|-----------------|
| `VendorID` | `tx.VendorID` | `"microsoft-*"` |
| `TargetURL` | `tx.TargetURL` (scheme stripped) | `"*.graph.microsoft.com/**"` |
| `EnvironmentID` | `tx.EnvironmentID` | `"prod-*"` |

### `Matches`

```go
func (r Route) Matches(tx sdk.TransactionContext) bool
```

Reports whether every non-empty field in the route matches the corresponding `tx` field. `TargetURL` matching strips the URL scheme (e.g., `https://`) before comparison.

### `Specificity`

```go
func (r Route) Specificity() int
```

Returns the number of non-empty fields (0–3). The mux prefers routes with higher specificity when multiple routes match.

| Route | Specificity |
|-------|-------------|
| `Route{}` | 0 |
| `Route{VendorID: "acme"}` | 1 |
| `Route{EnvironmentID: "prod", VendorID: "acme"}` | 2 |
| `Route{EnvironmentID: "prod", VendorID: "acme", TargetURL: "api.acme.com/**"}` | 3 |

### Glob patterns

Route fields use `GlobMatch(pattern, input, sep)` with `/` as the separator.

| Wildcard | Behavior | Example |
|----------|----------|---------|
| `*` | Matches within one segment. Does not cross separators. | `"microsoft-*"` matches `"microsoft-azure"` but not `"microsoft/azure"` |
| `**` | Matches across segments. Crosses separators. | `"*.graph.microsoft.com/**"` matches `"api.graph.microsoft.com/v1/users"` |

### `GlobMatch`

```go
func GlobMatch(pattern, input string, sep byte) bool
```

Package-level function. Tests whether `input` matches the glob `pattern` using `sep` as the segment separator. Route fields call this function internally with `/` as the separator. It can also be used directly for custom matching logic.

---

## OAuth2 client credentials

```go
import "github.com/cloudblue/chaperone/plugins/contrib/oauth"
```

Implements the OAuth2 client credentials grant (RFC 6749 Section 4.4).

### `ClientCredentialsConfig`

```go
type ClientCredentialsConfig struct {
    TokenURL     string
    ClientID     string
    ClientSecret string
    Scopes       []string
    ExtraParams  map[string]string
    AuthMode     AuthMode
    HTTPClient   *http.Client
    Logger       *slog.Logger
    ExpiryMargin time.Duration
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `TokenURL` | `string` | — (required) | OAuth2 token endpoint URL. |
| `ClientID` | `string` | — (required) | OAuth2 client identifier. |
| `ClientSecret` | `string` | — (required) | OAuth2 client secret. |
| `Scopes` | `[]string` | `nil` | Scopes to request. Joined with space per RFC 6749. |
| `ExtraParams` | `map[string]string` | `nil` | Extra form parameters merged into the token request. Cannot override standard fields (`grant_type`, `client_id`, `client_secret`, `scope`). |
| `AuthMode` | [`AuthMode`](#authmode) | `AuthModePost` | How credentials are sent to the token endpoint. |
| `HTTPClient` | [`*http.Client`][client] | 10s timeout, TLS 1.3+ | HTTP client for token requests. |
| `Logger` | [`*slog.Logger`][slog] | `slog.Default()` | Logger for debug and warning messages. |
| `ExpiryMargin` | [`time.Duration`][dur] | 1 minute | Subtracted from `expires_in` before setting `ExpiresAt`. |

### `AuthMode`

```go
type AuthMode int

const (
    AuthModePost  AuthMode = iota // client_secret_post (default)
    AuthModeBasic                 // client_secret_basic
)
```

| Value | Behavior |
|-------|----------|
| `AuthModePost` | Sends `client_id` and `client_secret` as form parameters in the POST body. |
| `AuthModeBasic` | Sends credentials via the `Authorization: Basic` header. |

### `NewClientCredentials`

```go
func NewClientCredentials(cfg ClientCredentialsConfig) *ClientCredentials
```

Creates a new client credentials provider. Applies defaults for unset optional fields (`HTTPClient`, `Logger`, `ExpiryMargin`). Does not validate required fields — an empty `TokenURL` or `ClientID` causes errors at first `GetCredentials` call, not at construction time.

### `ClientCredentials`

```go
type ClientCredentials struct{ /* unexported */ }
```

Implements [`CredentialProvider`][cp]. Safe for concurrent use.

#### `GetCredentials`

```go
func (cc *ClientCredentials) GetCredentials(ctx context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error)
```

Fetches an OAuth2 bearer token and returns a cacheable [`Credential`][cred] (Fast Path) with an `Authorization: Bearer` header.

**Behavior:**

- Returns a cached token if one exists and has not expired.
- On cache miss, fetches a new token from the token endpoint.
- Concurrent requests are deduplicated via singleflight — only one HTTP call is made regardless of how many goroutines call `GetCredentials` at the same time.
- `ExpiresAt` on the returned credential is `expires_in` minus the configured expiry margin.

---

## OAuth2 refresh token

```go
import "github.com/cloudblue/chaperone/plugins/contrib/oauth"
```

Implements the OAuth2 refresh token grant (RFC 6749 Section 6).

### `RefreshTokenConfig`

```go
type RefreshTokenConfig struct {
    TokenURL     string
    ClientID     string
    ClientSecret string
    Scopes       []string
    ExtraParams  map[string]string
    AuthMode     AuthMode
    Store        TokenStore
    HTTPClient   *http.Client
    Logger       *slog.Logger
    ExpiryMargin time.Duration
    OnSaveError  func(ctx context.Context, tokenURL string, err error)
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `TokenURL` | `string` | — (required) | OAuth2 token endpoint URL. |
| `ClientID` | `string` | — (required) | OAuth2 client identifier. |
| `ClientSecret` | `string` | — (required) | OAuth2 client secret. |
| `Scopes` | `[]string` | `nil` | Scopes to request. For v1-style endpoints, use `ExtraParams` with `"resource"` key instead. |
| `ExtraParams` | `map[string]string` | `nil` | Extra form parameters. Cannot override standard fields (`grant_type`, `client_id`, `client_secret`, `scope`, `refresh_token`). |
| `AuthMode` | [`AuthMode`](#authmode) | `AuthModePost` | How credentials are sent to the token endpoint. |
| `Store` | [`TokenStore`](#tokenstoreoauth) | — (required) | Refresh token persistence. |
| `HTTPClient` | [`*http.Client`][client] | 10s timeout, TLS 1.3+ | HTTP client for token requests. |
| `Logger` | [`*slog.Logger`][slog] | `slog.Default()` | Logger for debug, warning, and error messages. |
| `ExpiryMargin` | [`time.Duration`][dur] | 1 minute | Subtracted from `expires_in` before setting `ExpiresAt`. |
| `OnSaveError` | `func(ctx context.Context, tokenURL string, err error)` | `nil` | Optional callback invoked when a rotated refresh token fails to persist. Use for metrics or alerting. The request still succeeds with the access token; only logging occurs if nil. |

### `NewRefreshToken`

```go
func NewRefreshToken(cfg RefreshTokenConfig) *RefreshToken
```

Creates a new refresh token provider. Applies defaults for unset optional fields (`HTTPClient`, `Logger`, `ExpiryMargin`). Does not validate required fields — an empty `TokenURL` or nil `Store` causes errors at first `GetCredentials` call, not at construction time.

### `RefreshToken`

```go
type RefreshToken struct{ /* unexported */ }
```

Implements [`CredentialProvider`][cp]. Safe for concurrent use.

#### `GetCredentials`

```go
func (rt *RefreshToken) GetCredentials(ctx context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error)
```

Fetches an OAuth2 bearer token using the refresh token grant and returns a cacheable [`Credential`][cred] (Fast Path).

**Behavior:**

1. Returns a cached access token if one exists and has not expired.
2. On cache miss, loads the refresh token from the [`TokenStore`](#tokenstoreoauth).
3. Exchanges the refresh token at the token endpoint for a new access token.
4. If the response contains a rotated refresh token, saves it back to the store. A `Save` failure is logged at error level but does not fail the request — the access token is still valid for its TTL.
5. Concurrent requests are deduplicated via singleflight.

<a id="tokenstoreoauth"></a>

### OAuth `TokenStore`

```go
type TokenStore interface {
    Load(ctx context.Context) (refreshToken string, err error)
    Save(ctx context.Context, refreshToken string) error
}
```

Abstracts refresh token persistence for a single session. Scoped to one token URL, one client, one refresh token.

| Method | Description |
|--------|-------------|
| `Load` | Retrieves the current refresh token. |
| `Save` | Persists a rotated refresh token after a successful exchange. |

Implementations must be safe for concurrent use and should be durable. A failed `Save` after a successful exchange means the rotated refresh token is lost — the old one has been invalidated by the token endpoint.

---

## Microsoft Secure Application Model

```go
import "github.com/cloudblue/chaperone/plugins/contrib/microsoft"
```

Implements the delegated refresh token grant for Microsoft Partner Center. A single Azure AD app registration (one `ClientID` + `ClientSecret`) is shared across all tenants. Per-tenant state is managed by a keyed [`TokenStore`](#tokenstoremicrosoft).

### `Config`

```go
type Config struct {
    TokenEndpoint string
    ClientID      string
    ClientSecret  string
    Store         TokenStore
    MaxPoolSize   int
    ExpiryMargin  time.Duration
    HTTPClient    *http.Client
    Logger        *slog.Logger
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `TokenEndpoint` | `string` | `"https://login.microsoftonline.com"` | Base URL for the Microsoft token service. Override for sovereign clouds (e.g., `"https://login.microsoftonline.us"`). |
| `ClientID` | `string` | — (required) | Azure AD application (client) ID. |
| `ClientSecret` | `string` | — (required) | Azure AD application secret. |
| `Store` | [`TokenStore`](#tokenstoremicrosoft) | — (required) | Per-tenant, per-resource refresh token persistence. |
| `MaxPoolSize` | `int` | `10,000` | Maximum `oauth.RefreshToken` instances in the LRU pool. |
| `ExpiryMargin` | [`time.Duration`][dur] | 5 minutes | Subtracted from `expires_in` before setting `ExpiresAt`. Matches the Python connector's 300-second margin. |
| `HTTPClient` | [`*http.Client`][client] | 10s timeout, TLS 1.3+ | HTTP client for token requests. |
| `Logger` | [`*slog.Logger`][slog] | `slog.Default()` | Logger for debug, warning, and error messages. |

### `NewRefreshTokenSource`

```go
func NewRefreshTokenSource(cfg Config) *RefreshTokenSource
```

Creates a new Microsoft refresh token source. Applies defaults for unset optional fields (`TokenEndpoint`, `MaxPoolSize`, `ExpiryMargin`, `HTTPClient`, `Logger`). Does not validate required fields — an empty `ClientID` or nil `Store` causes errors at first `GetCredentials` call, not at construction time.

### `RefreshTokenSource`

```go
type RefreshTokenSource struct{ /* unexported */ }
```

Implements [`CredentialProvider`][cp]. Safe for concurrent use.

#### `GetCredentials`

```go
func (s *RefreshTokenSource) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error)
```

Extracts `TenantID` and `Resource` from [`tx.Data`][tx] and returns a cacheable [`Credential`][cred] (Fast Path).

**Context data contract:**

| Key | Type | Description |
|-----|------|-------------|
| `"TenantID"` | `string` | Azure AD tenant (e.g., `"contoso.onmicrosoft.com"`). Required. Returns [`ErrMissingContextData`](#sentinel-errors) if absent, [`ErrInvalidContextData`](#sentinel-errors) if not a string, empty, or contains invalid characters. Must match `^[a-zA-Z0-9][a-zA-Z0-9.\-]*$` (GUIDs, domain names, `common`/`organizations`/`consumers`). |
| `"Resource"` | `string` | Target resource (e.g., `"https://graph.microsoft.com"`). Required. Returns [`ErrMissingContextData`](#sentinel-errors) if absent, [`ErrInvalidContextData`](#sentinel-errors) if not a string or empty. |

**Token endpoint URL construction:**

```
{TokenEndpoint}/{TenantID}/oauth2/token
```

Example: `https://login.microsoftonline.com/contoso.onmicrosoft.com/oauth2/token`

The `resource` parameter is sent as a form parameter (v1 style), not as a v2 `scope`.

**Instance pool:**

Each unique `(TenantID, Resource)` pair gets its own `oauth.RefreshToken` instance with independent access token cache and singleflight group. Instances are kept in a bounded LRU pool:

- On access, the instance moves to the front.
- When the pool reaches `MaxPoolSize`, the least recently used instance is evicted. The refresh token remains safe in the `TokenStore` — only the in-memory access token cache is lost.

<a id="tokenstoremicrosoft"></a>

### Microsoft `TokenStore`

```go
type TokenStore interface {
    Load(ctx context.Context, tenantID, resource string) (refreshToken string, err error)
    Save(ctx context.Context, tenantID, resource string, refreshToken string) error
}
```

Multi-tenant refresh token persistence keyed by tenant and resource.

| Method | Parameters | Description |
|--------|-----------|-------------|
| `Load` | `tenantID`, `resource` | Retrieves the current refresh token for this tenant+resource pair. Returns [`ErrTenantNotFound`](#sentinel-errors) if no token exists. |
| `Save` | `tenantID`, `resource`, `refreshToken` | Persists a rotated refresh token after a successful exchange. |

Implementations must be safe for concurrent use and should be durable. A failed `Save` after a successful exchange means the rotated token is lost — the old one has been invalidated by Microsoft's token endpoint.

---

## Sentinel errors

All errors are defined in the `contrib` package and can be checked with [`errors.Is`](https://pkg.go.dev/errors#Is).

```go
import "github.com/cloudblue/chaperone/plugins/contrib"
```

| Error | Value | Cause | Retryable |
|-------|-------|-------|-----------|
| `ErrNoRouteMatch` | `"no route matched"` | No mux route matched and no default is configured. Proxy configuration issue. | No |
| `ErrMissingContextData` | `"missing required context data"` | Required key (`TenantID`, `Resource`) absent from `tx.Data`. Platform/caller issue. | No |
| `ErrInvalidContextData` | `"invalid context data type"` | Required key present but has wrong type (e.g., number instead of string), is an empty string, or contains invalid characters (TenantID must match `^[a-zA-Z0-9][a-zA-Z0-9.\-]*$`). Platform/caller issue. | No |
| `ErrTenantNotFound` | `"tenant not found"` | Tenant not in store or resolver. Proxy configuration issue. | No |
| `ErrInvalidCredentials` | `"invalid client credentials"` | OAuth2 token endpoint returned HTTP 401. Client secret is wrong or expired. | No |
| `ErrTokenExpiredOnArrival` | `"token expired on arrival"` | Token `expires_in` is less than or equal to the expiry margin. Token too short-lived to cache. | No |
| `ErrTokenEndpointUnavailable` | `"token endpoint unavailable"` | Network error, HTTP 5xx, or HTTP 429 from the token endpoint. | Yes |

---

## Adapter

### `AsPlugin`

```go
func AsPlugin(provider sdk.CredentialProvider) sdk.Plugin
```

Wraps a [`CredentialProvider`][cp] into a full [`Plugin`][plugin] with stub implementations:

- `GetCredentials` delegates to the wrapped provider.
- `SignCSR` returns an error (`"certificate signing not configured"`).
- `ModifyResponse` returns a nil action and nil error (no-op).

Use this to pass a building block to the [compliance test kit](sdk.md#compliance-test-kit):

```go
provider := oauth.NewClientCredentials(cfg)
compliance.VerifyContract(t, contrib.AsPlugin(provider))
```

The [`Mux`](#mux) implements `Plugin` directly and does not need this adapter.

---

## Examples

### Mux with Microsoft and generic OAuth2

```go
package main

import (
    "context"
    "os"

    "github.com/cloudblue/chaperone"
    "github.com/cloudblue/chaperone/plugins/contrib"
    "github.com/cloudblue/chaperone/plugins/contrib/microsoft"
    "github.com/cloudblue/chaperone/plugins/contrib/oauth"
)

func main() {
    mux := contrib.NewMux()

    // Microsoft vendors via Secure Application Model
    mux.Handle(
        contrib.Route{VendorID: "microsoft-*"},
        microsoft.NewRefreshTokenSource(microsoft.Config{
            ClientID:     os.Getenv("MS_CLIENT_ID"),
            ClientSecret: os.Getenv("MS_CLIENT_SECRET"),
            Store:        myVaultStore, // implements microsoft.TokenStore
        }),
    )

    // Generic OAuth2 vendor
    mux.Handle(
        contrib.Route{VendorID: "acme"},
        oauth.NewClientCredentials(oauth.ClientCredentialsConfig{
            TokenURL:     "https://auth.acme.com/oauth/token",
            ClientID:     os.Getenv("ACME_CLIENT_ID"),
            ClientSecret: os.Getenv("ACME_CLIENT_SECRET"),
            Scopes:       []string{"api.read", "api.write"},
        }),
    )

    // Fallback for unmatched vendors
    mux.Default(fileProvider)

    ctx := context.Background()
    chaperone.Run(ctx, mux)
}
```

### Vault-backed `TokenStore`

Skeleton implementation of [`microsoft.TokenStore`](#tokenstoremicrosoft) using HashiCorp Vault KV v2:

```go
type VaultTokenStore struct {
    client *vault.Client
    mount  string
}

func (v *VaultTokenStore) Load(ctx context.Context, tenantID, resource string) (string, error) {
    path := fmt.Sprintf("tenants/%s/%s", tenantID, resource)
    secret, err := v.client.KVv2(v.mount).Get(ctx, path)
    if err != nil {
        return "", fmt.Errorf("vault read %s: %w", path, err)
    }
    token, ok := secret.Data["refresh_token"].(string)
    if !ok {
        return "", contrib.ErrTenantNotFound
    }
    return token, nil
}

func (v *VaultTokenStore) Save(ctx context.Context, tenantID, resource, token string) error {
    path := fmt.Sprintf("tenants/%s/%s", tenantID, resource)
    _, err := v.client.KVv2(v.mount).Put(ctx, path, map[string]any{
        "refresh_token": token,
    })
    if err != nil {
        return fmt.Errorf("vault write %s: %w", path, err)
    }
    return nil
}
```
