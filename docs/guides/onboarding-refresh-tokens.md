# Onboard refresh tokens for OAuth2 and Microsoft SAM

How to bootstrap the initial refresh token for `oauth.RefreshToken` or `microsoft.RefreshTokenSource` using the `chaperone-onboard` CLI tool. This is a one-time step — once seeded, the proxy rotates tokens automatically.

> **Already have refresh tokens?** If you are migrating from an existing system and already hold valid refresh tokens, you can skip the CLI entirely. Seed your [`TokenStore`](#store-the-refresh-token) directly — write the token to the expected file path (or Vault key, database row, etc.) and start the proxy. The `chaperone-onboard` tool is only needed when you must perform the initial OAuth2 consent flow to obtain a refresh token for the first time.

## Prerequisites

### For Microsoft Secure Application Model (`microsoft` subcommand)

- An Azure AD app registration with the appropriate API permissions
- Admin consent capability for the target tenant
- The app's client ID and client secret
- (Optional) A target resource URI (e.g., `https://graph.microsoft.com`) to scope the initial consent

### For generic OAuth2 (`oauth` subcommand)

- The provider's authorization and token endpoint URLs
- Client ID and client secret (with offline access enabled)
- The required scopes (typically including `offline_access` or equivalent)

## Microsoft SAM walkthrough

**1. Set the client secret as an environment variable:**

```bash
export CHAPERONE_ONBOARD_CLIENT_SECRET='your-app-secret'
```

**2. Run the onboarding tool and authorize in the browser:**

```bash
chaperone-onboard microsoft \
  -tenant contoso.onmicrosoft.com \
  -client-id 12345678-abcd-1234-abcd-1234567890ab \
  > refresh-token.txt
```

The tool prints only the refresh token to stdout, so you can redirect to a file (as above) or pipe to your secrets manager. See [Store the refresh token](#store-the-refresh-token) for where to place the file.

The resulting refresh token is an MRRT (Multi-Resource Refresh Token) — one consent per tenant is sufficient for all resources. You can optionally pass `-resource https://graph.microsoft.com` to scope the initial consent to a specific resource.

At runtime, a [`KeyResolver`](../reference/contrib-plugins.md#keyresolver) can map transaction context fields to the correct tenant automatically.

The command opens your default browser to the Azure AD consent page. Sign in as an admin and grant consent. After granting consent, the browser shows a "You may close this tab and return to the terminal" page.

For sovereign clouds (Azure Government, Azure China), override the endpoint:

```bash
chaperone-onboard microsoft \
  -tenant contoso.onmicrosoft.us \
  -client-id 12345678-abcd-1234-abcd-1234567890ab \
  -endpoint https://login.microsoftonline.us
```

## Generic OAuth2 walkthrough

**1. Set the client secret:**

```bash
export CHAPERONE_ONBOARD_CLIENT_SECRET='your-client-secret'
```

**2. Run the onboarding tool and authorize in the browser:**

```bash
chaperone-onboard oauth \
  -authorize-url https://auth.vendor.com/authorize \
  -token-url https://auth.vendor.com/token \
  -client-id my-app \
  -scope "openid offline_access" \
  > refresh-token.txt
```

Stdout contains only the refresh token, so you can redirect it to a file or pipe it to a secrets manager. See [Store the refresh token](#store-the-refresh-token) for where to place the file.

The command opens your default browser to the provider's consent page. Complete the authorization flow and return to the terminal.

If your provider requires an exact redirect URI match, use a fixed port:

```bash
chaperone-onboard oauth \
  -authorize-url https://auth.vendor.com/authorize \
  -token-url https://auth.vendor.com/token \
  -client-id my-app \
  -scope "openid offline_access" \
  -port 8400
```

## Store the refresh token

The proxy reads the refresh token from a `TokenStore` at runtime ([OAuth variant](../reference/contrib-plugins.md#tokenstoreoauth), [Microsoft variant](../reference/contrib-plugins.md#tokenstoremicrosoft)). You need to seed the store with the token you just obtained.

### Using `FileStore` (built-in)

The contrib module ships file-based stores for both OAuth and Microsoft flows. Redirect the `chaperone-onboard` output to the expected file path.

**For generic OAuth2** — a single file per token:

```bash
chaperone-onboard oauth ... > /var/lib/chaperone/refresh-token.txt
```

Then configure the provider to read from that path:

```go
store := oauth.NewFileStore("/var/lib/chaperone/refresh-token.txt")
```

**For Microsoft SAM** — one file per tenant (MRRT model):

```bash
mkdir -p /var/lib/chaperone/tokens
chaperone-onboard microsoft \
  -tenant contoso.onmicrosoft.com \
  -client-id ... \
  > /var/lib/chaperone/tokens/contoso.onmicrosoft.com
```

Then point the store at the base directory:

```go
store := microsoft.NewFileStore("/var/lib/chaperone/tokens")
```

Each tenant gets a single file. At runtime, `FileStore.Save` calls `os.MkdirAll` and creates the base directory automatically when Microsoft rotates the token.

### Other backends

- **Vault:** Use the Vault CLI or API to write the token to the expected KV path. See the [Vault-backed skeleton](../reference/contrib-plugins.md#vault-backed-tokenstore) in the contrib reference.
- **Database:** Insert the token into the appropriate table.
- **Custom:** Implement the [`TokenStore`](../reference/contrib-plugins.md#tokenstoreoauth) interface (2 methods: `Load` and `Save`).

## Troubleshooting

### No `refresh_token` in response

The provider's token response did not include a refresh token. Common causes:

- **Missing `offline_access` scope.** Many providers require the `offline_access` scope to issue refresh tokens. Add `-scope "openid offline_access"` (or your provider's equivalent).
- **Provider-specific settings.** Some providers require enabling "offline access" or "refresh tokens" in the app registration or API console.

### Redirect URI mismatch

The provider rejected the callback because the redirect URI doesn't match what's registered in the app. Fix: use a fixed port with `-port` that matches your app registration's redirect URI (e.g., `http://127.0.0.1:8400/callback`).

### Sovereign cloud endpoints

For Azure Government, Azure China, or other sovereign clouds, use the `-endpoint` flag to override the default `https://login.microsoftonline.com`:

```bash
chaperone-onboard microsoft -endpoint https://login.microsoftonline.us ...
```

### PKCE errors with legacy providers

If your provider does not support PKCE (Proof Key for Code Exchange, RFC 7636), use `-no-pkce` with the `oauth` subcommand. The `microsoft` subcommand always uses PKCE (Azure AD v1 supports it).

### Headless environments (no browser)

Use `-no-browser` to print the authorization URL to stderr instead of opening a browser. Open the URL on a machine with a browser and complete the consent flow.

### Consent timeout

The default timeout is 5 minutes. If you need more time, use `-timeout 10m`.

### Testing against local servers without HTTPS

Both subcommands require HTTPS URLs by default. To test against a local mock server during development, pass the `-allow-http` flag. Never use it against real OAuth2 providers — credentials would be transmitted in plaintext.
