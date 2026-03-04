// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

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
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/cloudblue/chaperone/plugins/contrib"
)

// standardFields are form parameter names reserved by the OAuth2 spec.
// ExtraParams cannot override these, regardless of auth mode.
var standardFields = map[string]bool{
	"grant_type":    true,
	"client_id":     true,
	"client_secret": true,
	"scope":         true,
	"refresh_token": true,
}

// maxResponseBody limits how much of the token response we read (1 MB).
const maxResponseBody = 1 << 20

// oauthErrorResponse holds the standard error fields from an OAuth2 error
// response body (RFC 6749 Section 5.2). Only these fields are logged.
type oauthErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// tokenResponse represents the JSON body from an OAuth2 token endpoint.
type tokenResponse struct {
	AccessToken  string      `json:"access_token"` // #nosec G117 -- OAuth2 token response field
	ExpiresIn    json.Number `json:"expires_in"`
	TokenType    string      `json:"token_type"`
	RefreshToken string      `json:"refresh_token"` // #nosec G117 -- OAuth2 token response field
}

// tokenResult is the parsed and validated output of a token endpoint exchange.
type tokenResult struct {
	accessToken  string
	expiresAt    time.Time
	refreshToken string
}

// cachedToken holds a fetched access token and its computed expiration.
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// tokenManager handles caching and deduplicating token requests.
// The actual fetch logic is delegated to a grant-type-specific function.
type tokenManager struct {
	tokenURL  string
	logger    *slog.Logger
	fetchFunc func(ctx context.Context) (*cachedToken, error)

	mu    sync.RWMutex
	token *cachedToken

	group singleflight.Group
}

func newTokenManager(
	tokenURL string,
	logger *slog.Logger,
	fetchFunc func(context.Context) (*cachedToken, error),
) *tokenManager {
	return &tokenManager{
		tokenURL:  tokenURL,
		logger:    logger,
		fetchFunc: fetchFunc,
	}
}

// getToken returns a valid access token, fetching a new one if needed.
// Concurrent callers are deduplicated via singleflight.
func (tm *tokenManager) getToken(ctx context.Context) (string, time.Time, error) {
	tm.mu.RLock()
	if tm.token != nil && time.Now().Before(tm.token.expiresAt) {
		t := tm.token
		tm.mu.RUnlock()
		tm.logger.LogAttrs(ctx, slog.LevelDebug, "token cache hit",
			slog.String("token_url", tm.tokenURL))
		return t.accessToken, t.expiresAt, nil
	}
	tm.mu.RUnlock()

	tm.logger.LogAttrs(ctx, slog.LevelDebug, "token cache miss",
		slog.String("token_url", tm.tokenURL))

	// Use context.WithoutCancel so that a single caller's cancellation
	// does not poison all coalesced singleflight waiters.
	result, err, _ := tm.group.Do("token", func() (any, error) {
		cached, fetchErr := tm.fetchFunc(context.WithoutCancel(ctx))
		if fetchErr != nil {
			return nil, fetchErr
		}

		tm.mu.Lock()
		tm.token = cached
		tm.mu.Unlock()

		tm.logger.LogAttrs(ctx, slog.LevelDebug, "token fetched",
			slog.String("token_url", tm.tokenURL),
			slog.Time("expires_at", cached.expiresAt))

		return cached, nil
	})
	if err != nil {
		return "", time.Time{}, err
	}

	t, ok := result.(*cachedToken)
	if !ok {
		return "", time.Time{}, fmt.Errorf("unexpected singleflight result type %T", result)
	}
	return t.accessToken, t.expiresAt, nil
}

// tokenFetcher provides shared HTTP helpers for token endpoint communication.
// Both ClientCredentials and RefreshToken use a tokenFetcher to handle form
// encoding, auth modes, request execution, error classification, and response
// parsing.
type tokenFetcher struct {
	tokenURL     string
	clientID     string
	clientSecret string
	authMode     AuthMode
	scopes       []string
	extraParams  map[string]string
	expiryMargin time.Duration
	client       *http.Client
	logger       *slog.Logger
}

func newTokenFetcher(cfg tokenFetcher) *tokenFetcher {
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}
	if cfg.client == nil {
		cfg.client = defaultHTTPClient()
	}
	if cfg.expiryMargin == 0 {
		cfg.expiryMargin = defaultExpiryMargin
	}
	return &cfg
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		},
	}
}

// exchange sends a token request with the given form parameters and returns
// the parsed result. The caller is responsible for setting grant-type-specific
// form parameters (e.g., refresh_token) before calling exchange.
func (tf *tokenFetcher) exchange(ctx context.Context, form url.Values) (*tokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tf.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if tf.authMode == AuthModeBasic {
		req.SetBasicAuth(tf.clientID, tf.clientSecret)
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

	return tf.parseTokenResponse(body)
}

// buildForm constructs common form parameters for a token request.
// The grantType is set as grant_type. Grant-specific parameters (e.g.,
// refresh_token) must be added by the caller after buildForm returns.
func (tf *tokenFetcher) buildForm(grantType string) url.Values {
	form := url.Values{}
	form.Set("grant_type", grantType)

	if tf.authMode != AuthModeBasic {
		form.Set("client_id", tf.clientID)
		form.Set("client_secret", tf.clientSecret)
	}

	if len(tf.scopes) > 0 {
		form.Set("scope", strings.Join(tf.scopes, " "))
	}

	for k, v := range tf.extraParams {
		if !standardFields[k] {
			form.Set(k, v)
		}
	}

	return form
}

// doRequest executes the HTTP request and reads the response body.
func (tf *tokenFetcher) doRequest(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	resp, err := tf.client.Do(req) // #nosec G704 -- tokenURL is set by plugin author at construction, not from external input
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("token request for %s: %w", tf.tokenURL, ctx.Err())
		}
		tf.logger.LogAttrs(ctx, slog.LevelWarn, "token endpoint request failed",
			slog.String("token_url", tf.tokenURL),
			slog.String("error", err.Error()))
		return nil, nil, fmt.Errorf("token endpoint request for %s: %w",
			tf.tokenURL, contrib.ErrTokenEndpointUnavailable)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		_ = resp.Body.Close() // #nosec G104 -- best-effort close on read failure
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("reading token response for %s: %w", tf.tokenURL, ctx.Err())
		}
		return nil, nil, fmt.Errorf("reading token response from %s: %w",
			tf.tokenURL, contrib.ErrTokenEndpointUnavailable)
	}

	return resp, body, nil
}

// handleErrorResponse processes non-2xx responses from the token endpoint.
func (tf *tokenFetcher) handleErrorResponse(ctx context.Context, resp *http.Response, body []byte) error {
	contentType := resp.Header.Get("Content-Type")

	if tf.logger.Enabled(ctx, slog.LevelDebug) {
		attrs := []slog.Attr{
			slog.Int("status", resp.StatusCode),
			slog.String("content_type", contentType),
		}

		// Parse only the standard OAuth2 error fields (RFC 6749 §5.2).
		// Never log the raw body — it may contain echoed credentials.
		var oauthErr oauthErrorResponse
		if json.Unmarshal(body, &oauthErr) == nil && oauthErr.Error != "" {
			attrs = append(attrs, slog.String("oauth_error", oauthErr.Error))
			if oauthErr.Description != "" {
				attrs = append(attrs, slog.String("oauth_error_description", oauthErr.Description))
			}
		}

		tf.logger.LogAttrs(ctx, slog.LevelDebug, "token endpoint error response", attrs...)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("token endpoint returned %d (content-type: %s): %w",
			resp.StatusCode, contentType, contrib.ErrInvalidCredentials)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		tf.logger.LogAttrs(ctx, slog.LevelWarn, "token endpoint unavailable",
			slog.String("token_url", tf.tokenURL),
			slog.Int("status", resp.StatusCode))
		return fmt.Errorf("token endpoint returned %d (content-type: %s): %w",
			resp.StatusCode, contentType, contrib.ErrTokenEndpointUnavailable)
	}

	return fmt.Errorf("token endpoint returned %d (content-type: %s)",
		resp.StatusCode, contentType)
}

// parseTokenResponse unmarshals and validates the token endpoint response.
func (tf *tokenFetcher) parseTokenResponse(body []byte) (*tokenResult, error) {
	var tokenResp tokenResponse
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
	if expiresInDuration <= tf.expiryMargin {
		return nil, fmt.Errorf("token expires_in (%ds) <= expiry margin (%s): %w",
			expiresIn, tf.expiryMargin, contrib.ErrTokenExpiredOnArrival)
	}

	expiresAt := time.Now().Add(expiresInDuration - tf.expiryMargin)

	return &tokenResult{
		accessToken:  tokenResp.AccessToken,
		expiresAt:    expiresAt,
		refreshToken: tokenResp.RefreshToken,
	}, nil
}
