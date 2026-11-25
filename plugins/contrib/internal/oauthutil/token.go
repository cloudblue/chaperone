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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
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
	AccessToken string // #nosec G117 -- internal struct carrying parsed token data
	// ExpiresAt is a wall-clock instant: it carries no monotonic clock
	// reading, so comparing it with time.Now() uses the wall clock. See
	// ParseTokenResponse.
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
	if cfg.Client == nil {
		cfg.Client = DefaultHTTPClient()
	}
	if cfg.ExpiryMargin == 0 {
		cfg.ExpiryMargin = defaultExpiryMargin
	}
	return &cfg
}

// log returns the configured logger, or slog.Default() if none was set.
// Called at log-emit time so the current global default is always used
// when no explicit logger is provided.
func (tf *TokenFetcher) log() *slog.Logger {
	if tf.Logger != nil {
		return tf.Logger
	}
	return slog.Default()
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

	statusCode, header, body, err := tf.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if statusCode < 200 || statusCode >= 300 {
		return nil, tf.handleErrorResponse(ctx, statusCode, header, body)
	}

	ct := header.Get("Content-Type")
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

// doRequest executes the HTTP request, reads and closes the response body,
// and returns the status code, headers, and body bytes. The caller does not
// need to close anything.
func (tf *TokenFetcher) doRequest(ctx context.Context, req *http.Request) (statusCode int, header http.Header, body []byte, err error) {
	resp, err := tf.Client.Do(req) // #nosec G704 -- tokenURL is set by plugin author at construction, not from external input
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, nil, fmt.Errorf("token request for %s: %w", tf.TokenURL, ctx.Err())
		}
		tf.log().LogAttrs(ctx, slog.LevelWarn, "token endpoint request failed",
			slog.String("token_url", tf.TokenURL),
			slog.String("error", err.Error()))
		return 0, nil, nil, fmt.Errorf("token endpoint request for %s: %w",
			tf.TokenURL, contrib.ErrTokenEndpointUnavailable)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody))
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, nil, fmt.Errorf("reading token response for %s: %w", tf.TokenURL, ctx.Err())
		}
		return 0, nil, nil, fmt.Errorf("reading token response from %s: %w",
			tf.TokenURL, contrib.ErrTokenEndpointUnavailable)
	}

	return resp.StatusCode, resp.Header, body, nil
}

// safeErrorCode matches an OAuth2 error code that is safe to log and to put
// in an error message: the registered codes (invalid_grant, invalid_client,
// interaction_required, ...) are short identifiers. Anything else — free
// text, something long enough to be a credential — is not reported.
var safeErrorCode = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// providerErrorCode matches the error code Microsoft Entra ID puts at the
// start of error_description (e.g. "AADSTS50076: Due to a configuration
// change ..."). It identifies the cause far more precisely than the OAuth2
// error code, and carries no request or tenant data.
var providerErrorCode = regexp.MustCompile(`\bAADSTS\d{3,10}\b`)

// rejectionReason returns the parts of an OAuth2 error response that are
// safe to report at normal log levels: the standard error code and, when
// present, the identity provider's own error code. Either may be empty.
//
// error_description is deliberately left out: it is free text chosen by the
// identity provider and may echo request parameters or identify users and
// applications. It is only logged at debug level.
func rejectionReason(oauthErr OAuthErrorResponse) (code, providerCode string) {
	if safeErrorCode.MatchString(oauthErr.Error) {
		code = oauthErr.Error
	}
	return code, providerErrorCode.FindString(oauthErr.Description)
}

// handleErrorResponse processes non-2xx responses from the token endpoint.
//
// The reason the endpoint gave is reported at warn level and in the returned
// error, reduced to what rejectionReason considers safe. The response body
// itself is never logged.
func (tf *TokenFetcher) handleErrorResponse(ctx context.Context, statusCode int, header http.Header, body []byte) error {
	contentType := header.Get("Content-Type")

	// Parse only the standard OAuth2 error fields (RFC 6749 §5.2).
	// Never log the raw body — it may contain echoed credentials.
	var oauthErr OAuthErrorResponse
	if json.Unmarshal(body, &oauthErr) != nil {
		oauthErr = OAuthErrorResponse{}
	}

	if tf.log().Enabled(ctx, slog.LevelDebug) {
		attrs := []slog.Attr{
			slog.Int("status", statusCode),
			slog.String("content_type", contentType),
		}
		if oauthErr.Error != "" {
			attrs = append(attrs, slog.String("oauth_error", oauthErr.Error))
			if oauthErr.Description != "" {
				attrs = append(attrs, slog.String("oauth_error_description", oauthErr.Description))
			}
		}

		tf.log().LogAttrs(ctx, slog.LevelDebug, "token endpoint error response", attrs...)
	}

	code, providerCode := rejectionReason(oauthErr)

	attrs := []slog.Attr{
		slog.String("token_url", tf.TokenURL),
		slog.Int("status", statusCode),
	}
	detail := fmt.Sprintf("token endpoint returned %d (content-type: %s", statusCode, contentType)
	if code != "" {
		attrs = append(attrs, slog.String("oauth_error", code))
		detail += ", oauth_error: " + code
	}
	if providerCode != "" {
		attrs = append(attrs, slog.String("oauth_error_code", providerCode))
		detail += ", oauth_error_code: " + providerCode
	}
	detail += ")"

	switch {
	case statusCode == http.StatusUnauthorized:
		tf.log().LogAttrs(ctx, slog.LevelWarn, "token endpoint rejected the client credentials", attrs...)
		return fmt.Errorf("%s: %w", detail, contrib.ErrInvalidCredentials)

	case statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError:
		tf.log().LogAttrs(ctx, slog.LevelWarn, "token endpoint unavailable", attrs...)
		return fmt.Errorf("%s: %w", detail, contrib.ErrTokenEndpointUnavailable)

	default:
		tf.log().LogAttrs(ctx, slog.LevelWarn, "token endpoint rejected the token request", attrs...)
		return errors.New(detail)
	}
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

	// Round(0) strips the monotonic clock reading, so that every later
	// time.Now().Before(expiresAt) compares wall-clock time. A token expires
	// at a wall-clock instant, but the monotonic clock stops while the host
	// is suspended (laptop sleep, paused or frozen VM): with a monotonic
	// reading the token would be served past its real expiry for as long as
	// the suspension lasted.
	expiresAt := time.Now().Add(expiresInDuration - tf.ExpiryMargin).Round(0)

	return &TokenResult{
		AccessToken:  tokenResp.AccessToken,
		ExpiresAt:    expiresAt,
		RefreshToken: tokenResp.RefreshToken,
	}, nil
}
