// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauthutil

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIsStandardField(t *testing.T) {
	standard := []string{"grant_type", "client_id", "client_secret", "scope", "refresh_token"}
	for _, name := range standard {
		if !IsStandardField(name) {
			t.Errorf("IsStandardField(%q) = false, want true", name)
		}
	}

	nonStandard := []string{"resource", "audience", "tenant_id", "nonce", ""}
	for _, name := range nonStandard {
		if IsStandardField(name) {
			t.Errorf("IsStandardField(%q) = true, want false", name)
		}
	}
}

func TestNewTokenFetcher_Defaults(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{
		TokenURL: "https://example.com/token",
	})

	if f.Logger == nil {
		t.Error("Logger should default to non-nil")
	}
	if f.Client == nil {
		t.Error("Client should default to non-nil")
	}
	if f.ExpiryMargin != defaultExpiryMargin {
		t.Errorf("ExpiryMargin = %v, want %v", f.ExpiryMargin, defaultExpiryMargin)
	}
}

func TestNewTokenFetcher_PreservesExplicitValues(t *testing.T) {
	margin := 5 * time.Minute
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     "https://example.com/token",
		ExpiryMargin: margin,
	})

	if f.ExpiryMargin != margin {
		t.Errorf("ExpiryMargin = %v, want %v", f.ExpiryMargin, margin)
	}
}

func TestBuildForm_ClientCredentials_PostMode(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     "https://example.com/token",
		ClientID:     "my-id",
		ClientSecret: "my-secret",
		Scopes:       []string{"api.read", "api.write"},
		ExtraParams:  map[string]string{"resource": "https://graph.example.com"},
	})

	form := f.BuildForm("client_credentials")

	if got := form.Get("grant_type"); got != "client_credentials" {
		t.Errorf("grant_type = %q, want %q", got, "client_credentials")
	}
	if got := form.Get("client_id"); got != "my-id" {
		t.Errorf("client_id = %q, want %q", got, "my-id")
	}
	if got := form.Get("client_secret"); got != "my-secret" {
		t.Errorf("client_secret = %q, want %q", got, "my-secret")
	}
	if got := form.Get("scope"); got != "api.read api.write" {
		t.Errorf("scope = %q, want %q", got, "api.read api.write")
	}
	if got := form.Get("resource"); got != "https://graph.example.com" {
		t.Errorf("resource = %q, want %q", got, "https://graph.example.com")
	}
}

func TestBuildForm_BasicAuth_OmitsCredentials(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     "https://example.com/token",
		ClientID:     "my-id",
		ClientSecret: "my-secret",
		UseBasicAuth: true,
	})

	form := f.BuildForm("client_credentials")

	if form.Get("client_id") != "" {
		t.Error("client_id should not be in form when using basic auth")
	}
	if form.Get("client_secret") != "" {
		t.Error("client_secret should not be in form when using basic auth")
	}
}

func TestBuildForm_ExtraParams_CannotOverrideStandardFields(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     "https://example.com/token",
		ClientID:     "my-id",
		ClientSecret: "my-secret",
		ExtraParams: map[string]string{
			"grant_type":    "malicious_grant",
			"client_id":     "evil-id",
			"client_secret": "evil-secret",
			"scope":         "evil-scope",
			"refresh_token": "evil-token",
			"resource":      "https://allowed.example.com",
		},
	})

	form := f.BuildForm("client_credentials")

	if got := form.Get("grant_type"); got != "client_credentials" {
		t.Errorf("grant_type = %q, want %q (should not be overridden)", got, "client_credentials")
	}
	if got := form.Get("client_id"); got != "my-id" {
		t.Errorf("client_id = %q, want %q (should not be overridden)", got, "my-id")
	}
	if got := form.Get("resource"); got != "https://allowed.example.com" {
		t.Errorf("resource = %q, want %q (non-standard field should be included)", got, "https://allowed.example.com")
	}
}

func TestParseTokenResponse_Valid(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     "https://example.com/token",
		ExpiryMargin: 1 * time.Minute,
	})

	body, _ := json.Marshal(map[string]any{
		"access_token":  "at-123",
		"expires_in":    3600,
		"token_type":    "Bearer",
		"refresh_token": "rt-456",
	})

	result, err := f.ParseTokenResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccessToken != "at-123" {
		t.Errorf("AccessToken = %q, want %q", result.AccessToken, "at-123")
	}
	if result.RefreshToken != "rt-456" {
		t.Errorf("RefreshToken = %q, want %q", result.RefreshToken, "rt-456")
	}
	if time.Until(result.ExpiresAt) < 58*time.Minute || time.Until(result.ExpiresAt) > 60*time.Minute {
		t.Errorf("ExpiresAt should be ~59 minutes from now, got %v", time.Until(result.ExpiresAt))
	}
}

func TestParseTokenResponse_StringExpiresIn(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     "https://example.com/token",
		ExpiryMargin: 1 * time.Minute,
	})

	// Some providers return expires_in as a string.
	body := []byte(`{"access_token":"at","expires_in":"3600"}`)

	result, err := f.ParseTokenResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", result.AccessToken, "at")
	}
}

func TestParseTokenResponse_MissingAccessToken(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{TokenURL: "https://example.com/token"})

	body := []byte(`{"expires_in":3600}`)

	_, err := f.ParseTokenResponse(body)
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
}

func TestParseTokenResponse_NonPositiveExpiresIn(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{TokenURL: "https://example.com/token"})

	body := []byte(`{"access_token":"at","expires_in":0}`)

	_, err := f.ParseTokenResponse(body)
	if err == nil {
		t.Fatal("expected error for non-positive expires_in")
	}
}

func TestParseTokenResponse_ExpiresInLessThanMargin(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     "https://example.com/token",
		ExpiryMargin: 5 * time.Minute,
	})

	// 120 seconds < 5 minute margin
	body := []byte(`{"access_token":"at","expires_in":120}`)

	_, err := f.ParseTokenResponse(body)
	if err == nil {
		t.Fatal("expected error when expires_in <= expiry margin")
	}
}

func TestParseTokenResponse_InvalidJSON(t *testing.T) {
	f := NewTokenFetcher(TokenFetcher{TokenURL: "https://example.com/token"})

	_, err := f.ParseTokenResponse([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
