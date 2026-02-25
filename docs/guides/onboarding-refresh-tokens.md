# Onboarding refresh tokens

How to bootstrap the initial refresh token for `oauth.RefreshToken` or `microsoft.RefreshTokenSource` using the `chaperone-onboard` CLI tool. This is a one-time step — once seeded, the proxy rotates tokens automatically.

## Prerequisites

### For Microsoft Secure Application Model (`microsoft` subcommand)

- An Azure AD app registration with the appropriate API permissions
- Admin consent capability for the target tenant
- The app's client ID and client secret
- The target resource URI (e.g., `https://graph.microsoft.com`)

### For generic OAuth2 (`oauth` subcommand)

- The provider's authorization and token endpoint URLs
- Client ID and client secret (with offline access enabled)
- The required scopes (typically including `offline_access` or equivalent)

## Microsoft SAM walkthrough

**1. Set the client secret as an environment variable:**

```bash
export CHAPERONE_ONBOARD_CLIENT_SECRET='your-app-secret'
```

**2. Run the onboarding tool:**

```bash
chaperone-onboard microsoft \
  -tenant contoso.onmicrosoft.com \
  -client-id 12345678-abcd-1234-abcd-1234567890ab \
  -resource https://graph.microsoft.com \
  > refresh-token.txt
```

The tool prints only the refresh token to stdout, so you can redirect to a file (as above) or pipe to your secrets manager.

**3. Authorize in the browser.** The tool opens your default browser to the Azure AD consent page. Sign in as an admin and grant consent. After granting consent, the browser shows a "You may close this tab and return to the terminal" page.

For sovereign clouds (Azure Government, Azure China), override the endpoint:

```bash
chaperone-onboard microsoft \
  -tenant contoso.onmicrosoft.us \
  -client-id 12345678-abcd-1234-abcd-1234567890ab \
  -resource https://graph.microsoft.us \
  -endpoint https://login.microsoftonline.us
```

## Generic OAuth2 walkthrough

**1. Set the client secret:**

```bash
export CHAPERONE_ONBOARD_CLIENT_SECRET='your-client-secret'
```

**2. Run the onboarding tool:**

```bash
chaperone-onboard oauth \
  -authorize-url https://auth.vendor.com/authorize \
  -token-url https://auth.vendor.com/token \
  -client-id my-app \
  -scope "openid offline_access" \
  > refresh-token.txt
```

The tool prints only the refresh token to stdout, so you can redirect to a file (as above) or pipe to your secrets manager.

**3. Authorize in the browser** and complete the provider's consent flow.

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

Store the token in your `TokenStore` implementation. The proxy's `oauth.RefreshToken` and `microsoft.RefreshTokenSource` blocks read from it at startup. How you seed the store depends on your storage backend:

- **File-based store:** Write the token to the expected file path.
- **Vault-backed store:** Use the Vault CLI or API to write the token to the expected KV path.
- **Database-backed store:** Insert the token into the appropriate table.

See the [Contrib Plugins Reference](../reference/contrib-plugins.md) for the `TokenStore` interface definitions and a Vault-backed implementation skeleton.

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

If your provider does not support PKCE (RFC 7636), use `-no-pkce` with the `oauth` subcommand. The `microsoft` subcommand always uses PKCE (Azure AD v1 supports it).

### Headless environments (no browser)

Use `-no-browser` to print the authorization URL to stderr instead of opening a browser. Open the URL on a machine with a browser and complete the consent flow.

### Consent timeout

The default timeout is 5 minutes. If you need more time, use `-timeout 10m`.
