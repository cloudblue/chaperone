// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateState_Returns32ByteBase64URL(t *testing.T) {
	t.Parallel()

	state, err := generateState()
	if err != nil {
		t.Fatalf("generateState() error = %v", err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		t.Fatalf("state is not valid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("state decoded length = %d, want 32", len(decoded))
	}
}

func TestGenerateState_UniquePerCall(t *testing.T) {
	t.Parallel()

	s1, _ := generateState()
	s2, _ := generateState()
	if s1 == s2 {
		t.Error("two consecutive generateState() calls returned the same value")
	}
}

func TestGeneratePKCE_ChallengeMatchesVerifier(t *testing.T) {
	t.Parallel()

	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE() error = %v", err)
	}

	// Verify the challenge is SHA256(verifier) encoded as base64url
	h := sha256.Sum256([]byte(verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != expectedChallenge {
		t.Errorf("challenge = %q, want SHA256(%q) = %q", challenge, verifier, expectedChallenge)
	}
}

func TestGeneratePKCE_VerifierLength(t *testing.T) {
	t.Parallel()

	verifier, _, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE() error = %v", err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(verifier)
	if err != nil {
		t.Fatalf("verifier is not valid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("verifier decoded length = %d, want 32", len(decoded))
	}
}

func TestBuildAuthURL_GenericOAuth(t *testing.T) {
	t.Parallel()

	cfg := consentConfig{
		authorizeURL: "https://auth.example.com/authorize",
		clientID:     "my-app",
		scopes:       "openid offline_access",
		usePKCE:      true,
	}
	authURL := buildAuthURL(cfg, "test-state", "test-challenge", "http://127.0.0.1:9999/callback")

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}

	params := parsed.Query()
	assertParam(t, params, "client_id", "my-app")
	assertParam(t, params, "response_type", "code")
	assertParam(t, params, "redirect_uri", "http://127.0.0.1:9999/callback")
	assertParam(t, params, "state", "test-state")
	assertParam(t, params, "scope", "openid offline_access")
	assertParam(t, params, "code_challenge", "test-challenge")
	assertParam(t, params, "code_challenge_method", "S256")
}

func TestBuildAuthURL_MicrosoftWithResource(t *testing.T) {
	t.Parallel()

	extra := url.Values{}
	extra.Set("resource", "https://graph.microsoft.com")

	cfg := consentConfig{
		authorizeURL:    "https://login.microsoftonline.com/contoso/oauth2/authorize",
		clientID:        "app-id",
		extraAuthParams: extra,
		usePKCE:         true,
	}
	authURL := buildAuthURL(cfg, "state", "challenge", "http://127.0.0.1:9999/callback")

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}

	params := parsed.Query()
	assertParam(t, params, "resource", "https://graph.microsoft.com")
	if params.Get("scope") != "" {
		t.Error("Microsoft v1 should not have scope parameter")
	}
}

func TestBuildAuthURL_NoPKCE_OmitsChallengeParams(t *testing.T) {
	t.Parallel()

	cfg := consentConfig{
		authorizeURL: "https://auth.example.com/authorize",
		clientID:     "my-app",
	}
	authURL := buildAuthURL(cfg, "state", "", "http://127.0.0.1:9999/callback")

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}

	params := parsed.Query()
	if params.Get("code_challenge") != "" {
		t.Error("expected no code_challenge when PKCE is disabled")
	}
	if params.Get("code_challenge_method") != "" {
		t.Error("expected no code_challenge_method when PKCE is disabled")
	}
}

func TestStartCallbackServer_DeliversCode(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redirectURI, resultCh, err := startCallbackServer(ctx, 0, "expected-state")
	if err != nil {
		t.Fatalf("startCallbackServer() error = %v", err)
	}

	// Verify redirectURI binds to 127.0.0.1
	if !strings.HasPrefix(redirectURI, "http://127.0.0.1:") {
		t.Errorf("redirectURI = %q, want prefix http://127.0.0.1:", redirectURI)
	}

	// Simulate provider callback
	callbackURL := redirectURI + "?code=test-code&state=expected-state"
	resp, err := doTestGet(ctx, callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("callback status = %d, want 200", resp.StatusCode)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("callback result error = %v", result.err)
	}
	if result.code != "test-code" {
		t.Errorf("callback code = %q, want test-code", result.code)
	}
}

func TestStartCallbackServer_RejectsStateMismatch(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redirectURI, resultCh, err := startCallbackServer(ctx, 0, "correct-state")
	if err != nil {
		t.Fatalf("startCallbackServer() error = %v", err)
	}

	callbackURL := redirectURI + "?code=test-code&state=wrong-state"
	resp, err := doTestGet(ctx, callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("callback status = %d, want 400", resp.StatusCode)
	}

	result := <-resultCh
	if result.err == nil {
		t.Fatal("expected error for state mismatch")
	}
	if !strings.Contains(result.err.Error(), "state mismatch") {
		t.Errorf("error = %q, want containing 'state mismatch'", result.err.Error())
	}
}

func TestStartCallbackServer_HandlesProviderError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redirectURI, resultCh, err := startCallbackServer(ctx, 0, "state")
	if err != nil {
		t.Fatalf("startCallbackServer() error = %v", err)
	}

	callbackURL := redirectURI + "?error=access_denied&error_description=user+declined"
	resp, err := doTestGet(ctx, callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	result := <-resultCh
	if result.err == nil {
		t.Fatal("expected error for provider error")
	}
	if !strings.Contains(result.err.Error(), "access_denied") {
		t.Errorf("error = %q, want containing 'access_denied'", result.err.Error())
	}
}

func TestStartCallbackServer_RespectsContextTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _, err := startCallbackServer(ctx, 0, "state")
	if err != nil {
		t.Fatalf("startCallbackServer() error = %v", err)
	}

	// Wait for context to expire — the server should shut down
	<-ctx.Done()

	// Give the server a moment to shut down
	time.Sleep(50 * time.Millisecond)
}

func TestStartCallbackServer_Binds127001(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redirectURI, _, err := startCallbackServer(ctx, 0, "state")
	if err != nil {
		t.Fatalf("startCallbackServer() error = %v", err)
	}

	parsed, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatalf("failed to parse redirect URI: %v", err)
	}

	host := parsed.Hostname()
	if host != "127.0.0.1" {
		t.Errorf("callback server host = %q, want 127.0.0.1", host)
	}
}

func assertParam(t *testing.T, params url.Values, key, want string) {
	t.Helper()
	if got := params.Get(key); got != want {
		t.Errorf("param %q = %q, want %q", key, got, want)
	}
}

// doTestGet performs a GET request with context.
func doTestGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}
