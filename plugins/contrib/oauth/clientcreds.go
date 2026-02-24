// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package oauth provides generic OAuth2 building blocks for the Chaperone
// egress proxy.
//
// [ClientCredentials] implements the OAuth2 client credentials grant
// (RFC 6749 Section 4.4). [RefreshToken] implements the refresh token grant
// (RFC 6749 Section 6). Both implement [sdk.CredentialProvider] and handle
// token fetching, caching with expiry margin, and concurrent request
// deduplication via singleflight.
//
// Usage:
//
//	provider := oauth.NewClientCredentials(oauth.ClientCredentialsConfig{
//	    TokenURL:     "https://auth.vendor.com/oauth/token",
//	    ClientID:     "my-client-id",
//	    ClientSecret: "my-client-secret",
//	    Scopes:       []string{"api.read"},
//	})
//	cred, err := provider.GetCredentials(ctx, tx, req)
package oauth

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/cloudblue/chaperone/sdk"
)

// AuthMode determines how client credentials are sent to the token endpoint.
type AuthMode int

const (
	// AuthModePost sends client_id and client_secret as form parameters
	// in the POST body (client_secret_post). This is the default.
	AuthModePost AuthMode = iota

	// AuthModeBasic sends client credentials via the Authorization: Basic
	// header (client_secret_basic).
	AuthModeBasic
)

// defaultExpiryMargin is subtracted from the token's expires_in to prevent
// using tokens that are about to expire due to clock skew or network latency.
const defaultExpiryMargin = 1 * time.Minute

// ClientCredentialsConfig configures an OAuth2 client credentials grant.
type ClientCredentialsConfig struct {
	// TokenURL is the OAuth2 token endpoint URL.
	TokenURL string

	// ClientID is the OAuth2 client identifier.
	ClientID string

	// ClientSecret is the OAuth2 client secret.
	ClientSecret string // #nosec G117 -- config struct holds secrets by design

	// Scopes are the OAuth2 scopes to request. Joined with space per RFC 6749.
	Scopes []string

	// ExtraParams are additional form parameters merged into the token request
	// body. Standard fields (grant_type, client_id, client_secret, scope) take
	// precedence — ExtraParams cannot override them.
	ExtraParams map[string]string

	// AuthMode determines how credentials are sent. Default is AuthModePost.
	AuthMode AuthMode

	// HTTPClient is used for token requests. If nil, a default client with
	// 10s timeout and TLS 1.3 minimum is used.
	HTTPClient *http.Client

	// Logger for debug and warning messages. If nil, slog.Default() is used.
	Logger *slog.Logger

	// ExpiryMargin is subtracted from the token's expires_in before setting
	// ExpiresAt on the credential. Default is 1 minute.
	ExpiryMargin time.Duration
}

// Compile-time check that ClientCredentials implements CredentialProvider.
var _ sdk.CredentialProvider = (*ClientCredentials)(nil)

// ClientCredentials implements [sdk.CredentialProvider] using the OAuth2
// client credentials grant (RFC 6749 Section 4.4).
//
// It is safe for concurrent use from multiple goroutines.
type ClientCredentials struct {
	tm *tokenManager
}

// NewClientCredentials creates a new client credentials provider.
func NewClientCredentials(cfg ClientCredentialsConfig) *ClientCredentials {
	f := newTokenFetcher(tokenFetcher{
		tokenURL:     cfg.TokenURL,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		authMode:     cfg.AuthMode,
		scopes:       cfg.Scopes,
		extraParams:  cfg.ExtraParams,
		expiryMargin: cfg.ExpiryMargin,
		client:       cfg.HTTPClient,
		logger:       cfg.Logger,
	})

	fetchFunc := func(ctx context.Context) (*cachedToken, error) {
		form := f.buildForm("client_credentials")
		result, err := f.exchange(ctx, form)
		if err != nil {
			return nil, err
		}
		return &cachedToken{
			accessToken: result.accessToken,
			expiresAt:   result.expiresAt,
		}, nil
	}

	return &ClientCredentials{
		tm: newTokenManager(cfg.TokenURL, f.logger, fetchFunc),
	}
}

// GetCredentials fetches an OAuth2 bearer token and returns it as a
// cacheable credential (Fast Path).
//
// The returned credential contains an Authorization: Bearer header and an
// ExpiresAt time adjusted by the configured expiry margin.
func (cc *ClientCredentials) GetCredentials(ctx context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	token, expiresAt, err := cc.tm.getToken(ctx)
	if err != nil {
		return nil, err
	}

	return &sdk.Credential{
		Headers:   map[string]string{"Authorization": "Bearer " + token},
		ExpiresAt: expiresAt,
	}, nil
}
