// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package oauthutil provides shared HTTP helpers for OAuth2 token endpoint
// communication. It is used by both the generic oauth package and the
// Microsoft-specific building block to avoid duplicating HTTP exchange,
// form building, error classification, and response parsing logic.
package oauthutil

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudblue/chaperone/plugins/contrib"
)

// IsStandardField reports whether the given form parameter name is reserved
// by the OAuth2 spec. ExtraParams cannot override these, regardless of auth mode.
func IsStandardField(name string) bool {
	switch name {
	case "grant_type", "client_id", "client_secret", "scope", "refresh_token":
		return true
	}
	return false
}

// MaxResponseBody limits how much of the token response we read (1 MB).
const MaxResponseBody = 1 << 20

// OAuthErrorResponse holds the standard error fields from an OAuth2 error
// response body (RFC 6749 Section 5.2). Only these fields are logged.
type OAuthErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// TokenResponse represents the JSON body from an OAuth2 token endpoint.
type TokenResponse struct {
	AccessToken  string      `json:"access_token"` // #nosec G117 -- OAuth2 token response field
	ExpiresIn    json.Number `json:"expires_in"`
	TokenType    string      `json:"token_type"`
	RefreshToken string      `json:"refresh_token"` // #nosec G117 -- OAuth2 token response field
}

// TokenResult is the parsed and validated output of a token endpoint exchange.
type TokenResult struct {
	AccessToken  string // #nosec G117 -- internal struct carrying parsed token data
	ExpiresAt    time.Time
	RefreshToken string // #nosec G117 -- internal struct carrying parsed token data
}

// DefaultHTTPClient returns an HTTP client with 10s timeout and TLS 1.3 minimum.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		},
	}
}

// TokenFetcher provides shared HTTP helpers for token endpoint communication.
// Both ClientCredentials and RefreshToken use a TokenFetcher to handle form
// encoding, auth modes, request execution, error classification, and response
// parsing.
type TokenFetcher struct {
	TokenURL     string
	ClientID     string
	ClientSecret string // #nosec G117 -- required config field for OAuth2 client authentication
	UseBasicAuth bool
	Scopes       []string
	ExtraParams  map[string]string
	ExpiryMargin time.Duration
	Client       *http.Client
	Logger       *slog.Logger
}

// defaultExpiryMargin is subtracted from the token's expires_in to prevent
// using tokens that are about to expire due to clock skew or network latency.
const defaultExpiryMargin = 1 * time.Minute

// NewTokenFetcher creates a TokenFetcher with defaults applied for nil fields.
func NewTokenFetcher(cfg TokenFetcher) *TokenFetcher {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Client == nil {
		cfg.Client = DefaultHTTPClient()
	}
	if cfg.ExpiryMargin == 0 {
		cfg.ExpiryMargin = defaultExpiryMargin
	}
	return &cfg
}

// Exchange sends a token request with the given form parameters and returns
// the parsed result. The caller is responsible for setting grant-type-specific
// form parameters (e.g., refresh_token) before calling Exchange.
func (tf *TokenFetcher) Exchange(ctx context.Context, form url.Values) (*TokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tf.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if tf.UseBasicAuth {
		req.SetBasicAuth(tf.ClientID, tf.ClientSecret)
	}

	resp, body, err := tf.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, tf.handleErrorResponse(ctx, resp, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return nil, fmt.Errorf("unexpected token response content-type: %s", ct)
	}

	return tf.ParseTokenResponse(body)
}

// BuildForm constructs common form parameters for a token request.
// The grantType is set as grant_type. Grant-specific parameters (e.g.,
// refresh_token) must be added by the caller after BuildForm returns.
func (tf *TokenFetcher) BuildForm(grantType string) url.Values {
	form := url.Values{}
	form.Set("grant_type", grantType)

	if !tf.UseBasicAuth {
		form.Set("client_id", tf.ClientID)
		form.Set("client_secret", tf.ClientSecret)
	}

	if len(tf.Scopes) > 0 {
		form.Set("scope", strings.Join(tf.Scopes, " "))
	}

	for k, v := range tf.ExtraParams {
		if !IsStandardField(k) {
			form.Set(k, v)
		}
	}

	return form
}

// doRequest executes the HTTP request and reads the response body.
func (tf *TokenFetcher) doRequest(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	resp, err := tf.Client.Do(req) // #nosec G704 -- tokenURL is set by plugin author at construction, not from external input
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("token request for %s: %w", tf.TokenURL, ctx.Err())
		}
		tf.Logger.LogAttrs(ctx, slog.LevelWarn, "token endpoint request failed",
			slog.String("token_url", tf.TokenURL),
			slog.String("error", err.Error()))
		return nil, nil, fmt.Errorf("token endpoint request for %s: %w",
			tf.TokenURL, contrib.ErrTokenEndpointUnavailable)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody))
	if err != nil {
		_ = resp.Body.Close() // #nosec G104 -- best-effort close on read failure
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("reading token response for %s: %w", tf.TokenURL, ctx.Err())
		}
		return nil, nil, fmt.Errorf("reading token response from %s: %w",
			tf.TokenURL, contrib.ErrTokenEndpointUnavailable)
	}

	return resp, body, nil
}

// handleErrorResponse processes non-2xx responses from the token endpoint.
func (tf *TokenFetcher) handleErrorResponse(ctx context.Context, resp *http.Response, body []byte) error {
	contentType := resp.Header.Get("Content-Type")

	if tf.Logger.Enabled(ctx, slog.LevelDebug) {
		attrs := []slog.Attr{
			slog.Int("status", resp.StatusCode),
			slog.String("content_type", contentType),
		}

		// Parse only the standard OAuth2 error fields (RFC 6749 §5.2).
		// Never log the raw body — it may contain echoed credentials.
		var oauthErr OAuthErrorResponse
		if json.Unmarshal(body, &oauthErr) == nil && oauthErr.Error != "" {
			attrs = append(attrs, slog.String("oauth_error", oauthErr.Error))
			if oauthErr.Description != "" {
				attrs = append(attrs, slog.String("oauth_error_description", oauthErr.Description))
			}
		}

		tf.Logger.LogAttrs(ctx, slog.LevelDebug, "token endpoint error response", attrs...)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("token endpoint returned %d (content-type: %s): %w",
			resp.StatusCode, contentType, contrib.ErrInvalidCredentials)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		tf.Logger.LogAttrs(ctx, slog.LevelWarn, "token endpoint unavailable",
			slog.String("token_url", tf.TokenURL),
			slog.Int("status", resp.StatusCode))
		return fmt.Errorf("token endpoint returned %d (content-type: %s): %w",
			resp.StatusCode, contentType, contrib.ErrTokenEndpointUnavailable)
	}

	return fmt.Errorf("token endpoint returned %d (content-type: %s)",
		resp.StatusCode, contentType)
}

// ParseTokenResponse unmarshals and validates the token endpoint response.
func (tf *TokenFetcher) ParseTokenResponse(body []byte) (*TokenResult, error) {
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}

	expiresIn, err := tokenResp.ExpiresIn.Int64()
	if err != nil {
		return nil, fmt.Errorf("parsing expires_in %q: %w", tokenResp.ExpiresIn, err)
	}
	if expiresIn <= 0 {
		return nil, fmt.Errorf("token response missing or non-positive expires_in")
	}

	expiresInDuration := time.Duration(expiresIn) * time.Second
	if expiresInDuration <= tf.ExpiryMargin {
		return nil, fmt.Errorf("token expires_in (%ds) <= expiry margin (%s): %w",
			expiresIn, tf.ExpiryMargin, contrib.ErrTokenExpiredOnArrival)
	}

	expiresAt := time.Now().Add(expiresInDuration - tf.ExpiryMargin)

	return &TokenResult{
		AccessToken:  tokenResp.AccessToken,
		ExpiresAt:    expiresAt,
		RefreshToken: tokenResp.RefreshToken,
	}, nil
}
