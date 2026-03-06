// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBody limits how much of the token response we read (1 MB).
const maxResponseBody = 1 << 20

// exchangeConfig holds the parameters for a token exchange.
type exchangeConfig struct {
	tokenURL     string
	clientID     string
	clientSecret string
	code         string
	redirectURI  string
	codeVerifier string       // Empty if PKCE disabled
	extraParams  url.Values   // Extra form params (e.g., resource)
	client       *http.Client // Optional; if nil, uses a secure default
}

// oauthTokenResponse holds the JSON fields from a token endpoint response.
type oauthTokenResponse struct {
	RefreshToken string `json:"refresh_token"` // #nosec G117 -- OAuth2 response field, not a credential
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// exchangeCode performs the OAuth2 authorization code exchange and returns
// the refresh token from the response.
func exchangeCode(ctx context.Context, cfg exchangeConfig) (string, error) {
	form := buildExchangeForm(cfg)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := cfg.client
	if client == nil {
		client = defaultExchangeClient()
	}

	resp, err := client.Do(req) // #nosec G704 -- tokenURL is set by CLI flags, not external input
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", parseErrorResponse(resp.StatusCode, body)
	}

	return parseRefreshToken(body)
}

// buildExchangeForm constructs the form parameters for the token exchange.
func buildExchangeForm(cfg exchangeConfig) url.Values {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.clientID)
	form.Set("client_secret", cfg.clientSecret)
	form.Set("code", cfg.code)
	form.Set("redirect_uri", cfg.redirectURI)

	if cfg.codeVerifier != "" {
		form.Set("code_verifier", cfg.codeVerifier)
	}

	for k, vs := range cfg.extraParams {
		for _, v := range vs {
			form.Set(k, v)
		}
	}

	return form
}

// defaultExchangeClient returns an HTTP client with TLS 1.3 minimum and 30s timeout.
func defaultExchangeClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		},
	}
}

// parseRefreshToken extracts the refresh token from a successful token response.
func parseRefreshToken(body []byte) (string, error) {
	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.RefreshToken == "" {
		return "", fmt.Errorf("no refresh_token in response — ensure your provider is configured " +
			"for offline access (e.g., scope 'offline_access' or provider-specific offline access setting)")
	}

	return tokenResp.RefreshToken, nil
}

// parseErrorResponse extracts the OAuth2 error fields (RFC 6749 §5.2) from
// an error response. It never logs the raw body.
func parseErrorResponse(statusCode int, body []byte) error {
	var oauthErr oauthTokenResponse
	if json.Unmarshal(body, &oauthErr) == nil && oauthErr.Error != "" {
		if oauthErr.Description != "" {
			return fmt.Errorf("token exchange failed (HTTP %d): %s: %s",
				statusCode, oauthErr.Error, oauthErr.Description)
		}
		return fmt.Errorf("token exchange failed (HTTP %d): %s", statusCode, oauthErr.Error)
	}
	return fmt.Errorf("token exchange failed (HTTP %d)", statusCode)
}
