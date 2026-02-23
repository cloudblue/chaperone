// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/cloudblue/chaperone/sdk"
)

// RefreshTokenConfig configures an OAuth2 refresh token grant.
type RefreshTokenConfig struct {
	// TokenURL is the OAuth2 token endpoint URL.
	TokenURL string

	// ClientID is the OAuth2 client identifier.
	ClientID string

	// ClientSecret is the OAuth2 client secret.
	ClientSecret string

	// Scopes are the OAuth2 scopes to request. Joined with space per RFC 6749.
	// For v1-style endpoints, use ExtraParams with "resource" key instead.
	Scopes []string

	// ExtraParams are additional form parameters merged into the token request
	// body. Standard fields (grant_type, client_id, client_secret, scope,
	// refresh_token) take precedence — ExtraParams cannot override them.
	ExtraParams map[string]string

	// AuthMode determines how credentials are sent. Default is AuthModePost.
	AuthMode AuthMode

	// Store provides refresh token persistence. Required.
	Store TokenStore

	// HTTPClient is used for token requests. If nil, a default client with
	// 10s timeout and TLS 1.3 minimum is used.
	HTTPClient *http.Client

	// Logger for debug, warning, and error messages. If nil, slog.Default()
	// is used.
	Logger *slog.Logger

	// ExpiryMargin is subtracted from the token's expires_in before setting
	// ExpiresAt on the credential. Default is 1 minute.
	ExpiryMargin time.Duration

	// OnSaveError is an optional callback invoked when a rotated refresh
	// token fails to persist. This allows operators to hook metrics or
	// alerting for this degraded state. The current request still succeeds
	// (returning the access token), but subsequent refreshes may fail if the
	// old token has been invalidated by the IdP.
	// If nil, only logging occurs.
	OnSaveError func(ctx context.Context, tokenURL string, err error)
}

// Compile-time check that RefreshToken implements CredentialProvider.
var _ sdk.CredentialProvider = (*RefreshToken)(nil)

// RefreshToken implements [sdk.CredentialProvider] using the OAuth2 refresh
// token grant (RFC 6749 Section 6).
//
// It loads the current refresh token from a [TokenStore], exchanges it at the
// token endpoint for an access token, and saves any rotated refresh token back
// to the store. Access tokens are cached internally with expiry margin and
// concurrent requests are deduplicated via singleflight.
//
// It is safe for concurrent use from multiple goroutines.
type RefreshToken struct {
	tm          *tokenManager
	fetcher     *tokenFetcher
	store       TokenStore
	logger      *slog.Logger
	onSaveError func(ctx context.Context, tokenURL string, err error)
}

// NewRefreshToken creates a new refresh token provider.
func NewRefreshToken(cfg RefreshTokenConfig) *RefreshToken {
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

	rt := &RefreshToken{
		fetcher:     f,
		store:       cfg.Store,
		logger:      f.logger,
		onSaveError: cfg.OnSaveError,
	}

	rt.tm = newTokenManager(cfg.TokenURL, f.logger, rt.fetch)

	return rt
}

// GetCredentials fetches an OAuth2 bearer token using the refresh token grant
// and returns it as a cacheable credential (Fast Path).
//
// The returned credential contains an Authorization: Bearer header and an
// ExpiresAt time adjusted by the configured expiry margin.
func (rt *RefreshToken) GetCredentials(ctx context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	token, expiresAt, err := rt.tm.getToken(ctx)
	if err != nil {
		return nil, err
	}

	return &sdk.Credential{
		Headers:   map[string]string{"Authorization": "Bearer " + token},
		ExpiresAt: expiresAt,
	}, nil
}

// fetch loads the refresh token from the store, exchanges it at the token
// endpoint, and saves any rotated refresh token back.
func (rt *RefreshToken) fetch(ctx context.Context) (*cachedToken, error) {
	refreshToken, err := rt.store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading refresh token: %w", err)
	}

	form := rt.fetcher.buildForm("refresh_token")
	form.Set("refresh_token", refreshToken)

	result, err := rt.fetcher.exchange(ctx, form)
	if err != nil {
		return nil, err
	}

	if result.refreshToken != "" {
		if saveErr := rt.store.Save(ctx, result.refreshToken); saveErr != nil {
			rt.logger.LogAttrs(ctx, slog.LevelError, "failed to save rotated refresh token",
				slog.String("token_url", rt.fetcher.tokenURL),
				slog.String("error", saveErr.Error()))

			if rt.onSaveError != nil {
				rt.onSaveError(ctx, rt.fetcher.tokenURL, saveErr)
			}
		}
	}

	return &cachedToken{
		accessToken: result.accessToken,
		expiresAt:   result.expiresAt,
	}, nil
}
