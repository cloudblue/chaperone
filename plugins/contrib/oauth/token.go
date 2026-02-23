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

	"github.com/cloudblue/chaperone/plugins/contrib"
	"golang.org/x/sync/singleflight"
)

// standardFields are form parameter names reserved by the OAuth2 spec.
// ExtraParams cannot override these, regardless of auth mode.
var standardFields = map[string]bool{
	"grant_type":    true,
	"client_id":     true,
	"client_secret": true,
	"scope":         true,
}

// maxResponseBody limits how much of the token response we read (1 MB).
const maxResponseBody = 1 << 20

// debugBodyPrefix is the maximum number of bytes from an error response body
// included in debug log output.
const debugBodyPrefix = 256

// tokenResponse represents the JSON body from an OAuth2 token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// cachedToken holds a fetched access token and its computed expiration.
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// tokenManager handles fetching, caching, and deduplicating token requests.
type tokenManager struct {
	cfg    ClientCredentialsConfig
	logger *slog.Logger
	client *http.Client

	mu    sync.RWMutex
	token *cachedToken

	group singleflight.Group
}

func newTokenManager(cfg ClientCredentialsConfig) *tokenManager {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	client := cfg.HTTPClient
	if client == nil {
		client = defaultHTTPClient()
	}

	return &tokenManager{
		cfg:    cfg,
		logger: logger,
		client: client,
	}
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

// getToken returns a valid access token, fetching a new one if needed.
// Concurrent callers are deduplicated via singleflight.
func (tm *tokenManager) getToken(ctx context.Context) (string, time.Time, error) {
	tm.mu.RLock()
	if tm.token != nil && time.Now().Before(tm.token.expiresAt) {
		t := tm.token
		tm.mu.RUnlock()
		tm.logger.LogAttrs(ctx, slog.LevelDebug, "token cache hit",
			slog.String("token_url", tm.cfg.TokenURL))
		return t.accessToken, t.expiresAt, nil
	}
	tm.mu.RUnlock()

	tm.logger.LogAttrs(ctx, slog.LevelDebug, "token cache miss",
		slog.String("token_url", tm.cfg.TokenURL))

	result, err, _ := tm.group.Do("token", func() (any, error) {
		return tm.fetchToken(ctx)
	})
	if err != nil {
		return "", time.Time{}, err
	}

	t := result.(*cachedToken)
	return t.accessToken, t.expiresAt, nil
}

// fetchToken performs the HTTP request to the token endpoint.
func (tm *tokenManager) fetchToken(ctx context.Context) (*cachedToken, error) {
	form := tm.buildForm()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tm.cfg.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if tm.cfg.AuthMode == AuthModeBasic {
		req.SetBasicAuth(tm.cfg.ClientID, tm.cfg.ClientSecret)
	}

	resp, body, err := tm.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, tm.handleErrorResponse(ctx, resp, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return nil, fmt.Errorf("unexpected token response content-type: %s", ct)
	}

	return tm.parseTokenResponse(ctx, body)
}

// doRequest executes the HTTP request and reads the response body.
func (tm *tokenManager) doRequest(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	resp, err := tm.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("token request for %s: %w", tm.cfg.TokenURL, ctx.Err())
		}
		tm.logger.LogAttrs(ctx, slog.LevelWarn, "token endpoint request failed",
			slog.String("token_url", tm.cfg.TokenURL),
			slog.String("error", err.Error()))
		return nil, nil, fmt.Errorf("token endpoint request for %s: %w",
			tm.cfg.TokenURL, contrib.ErrTokenEndpointUnavailable)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		resp.Body.Close()
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("reading token response for %s: %w", tm.cfg.TokenURL, ctx.Err())
		}
		return nil, nil, fmt.Errorf("reading token response from %s: %w",
			tm.cfg.TokenURL, contrib.ErrTokenEndpointUnavailable)
	}

	return resp, body, nil
}

// buildForm constructs the form parameters for the token request.
func (tm *tokenManager) buildForm() url.Values {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	if tm.cfg.AuthMode != AuthModeBasic {
		form.Set("client_id", tm.cfg.ClientID)
		form.Set("client_secret", tm.cfg.ClientSecret)
	}

	if len(tm.cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(tm.cfg.Scopes, " "))
	}

	for k, v := range tm.cfg.ExtraParams {
		if !standardFields[k] {
			form.Set(k, v)
		}
	}

	return form
}

// handleErrorResponse processes non-2xx responses from the token endpoint.
func (tm *tokenManager) handleErrorResponse(ctx context.Context, resp *http.Response, body []byte) error {
	contentType := resp.Header.Get("Content-Type")

	if tm.logger.Enabled(ctx, slog.LevelDebug) {
		prefix := string(body)
		if len(prefix) > debugBodyPrefix {
			prefix = prefix[:debugBodyPrefix]
		}
		tm.logger.LogAttrs(ctx, slog.LevelDebug, "token endpoint error response",
			slog.Int("status", resp.StatusCode),
			slog.String("content_type", contentType),
			slog.String("body_prefix", prefix))
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("token endpoint returned %d (content-type: %s): %w",
			resp.StatusCode, contentType, contrib.ErrInvalidCredentials)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		tm.logger.LogAttrs(ctx, slog.LevelWarn, "token endpoint unavailable",
			slog.String("token_url", tm.cfg.TokenURL),
			slog.Int("status", resp.StatusCode))
		return fmt.Errorf("token endpoint returned %d (content-type: %s): %w",
			resp.StatusCode, contentType, contrib.ErrTokenEndpointUnavailable)
	}

	return fmt.Errorf("token endpoint returned %d (content-type: %s)",
		resp.StatusCode, contentType)
}

// parseTokenResponse unmarshals and validates the token endpoint response.
func (tm *tokenManager) parseTokenResponse(ctx context.Context, body []byte) (*cachedToken, error) {
	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}

	if tokenResp.ExpiresIn == 0 {
		return nil, fmt.Errorf("token response missing expires_in")
	}

	margin := tm.cfg.ExpiryMargin
	if margin == 0 {
		margin = defaultExpiryMargin
	}

	expiresInDuration := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiresInDuration <= margin {
		return nil, fmt.Errorf("token expires_in (%ds) <= expiry margin (%s): %w",
			tokenResp.ExpiresIn, margin, contrib.ErrTokenExpiredOnArrival)
	}

	expiresAt := time.Now().Add(expiresInDuration - margin)

	cached := &cachedToken{
		accessToken: tokenResp.AccessToken,
		expiresAt:   expiresAt,
	}

	tm.mu.Lock()
	tm.token = cached
	tm.mu.Unlock()

	tm.logger.LogAttrs(ctx, slog.LevelDebug, "token fetched",
		slog.String("token_url", tm.cfg.TokenURL),
		slog.Time("expires_at", expiresAt))

	return cached, nil
}
