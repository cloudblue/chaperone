// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestExchangeCode_ValidResponse_ReturnsRefreshToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := r.FormValue("client_id"); got != "my-app" {
			t.Errorf("client_id = %q, want my-app", got)
		}
		if got := r.FormValue("client_secret"); got != "s3cret" {
			t.Errorf("client_secret = %q, want s3cret", got)
		}
		if got := r.FormValue("code"); got != "auth-code-123" {
			t.Errorf("code = %q, want auth-code-123", got)
		}
		if got := r.FormValue("redirect_uri"); got != "http://127.0.0.1:9999/callback" {
			t.Errorf("redirect_uri = %q, want http://127.0.0.1:9999/callback", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at","refresh_token":"rt-456","expires_in":3600}`)) //nolint:errcheck
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	token, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "my-app",
		clientSecret: "s3cret",
		code:         "auth-code-123",
		redirectURI:  "http://127.0.0.1:9999/callback",
		client:       server.Client(),
	})
	if err != nil {
		t.Fatalf("exchangeCode() error = %v", err)
	}
	if token != "rt-456" {
		t.Errorf("refresh token = %q, want rt-456", token)
	}
}

func TestExchangeCode_MissingRefreshToken_ReturnsActionableError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at","expires_in":3600}`)) //nolint:errcheck
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "my-app",
		clientSecret: "secret",
		code:         "code",
		redirectURI:  "http://127.0.0.1:9999/callback",
		client:       server.Client(),
	})
	if err == nil {
		t.Fatal("expected error for missing refresh_token")
	}
	want := "no refresh_token in response"
	if got := err.Error(); !strings.Contains(got, want) {
		t.Errorf("error = %q, want containing %q", got, want)
	}
}

func TestExchangeCode_HTTPError_ReturnsOAuthError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client","error_description":"bad credentials"}`)) //nolint:errcheck
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "my-app",
		clientSecret: "wrong",
		code:         "code",
		redirectURI:  "http://127.0.0.1:9999/callback",
		client:       server.Client(),
	})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if got := err.Error(); !strings.Contains(got, "invalid_client") || !strings.Contains(got, "bad credentials") {
		t.Errorf("error = %q, want containing oauth error details", got)
	}
}

func TestExchangeCode_ServerError_ReturnsGenericError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "my-app",
		clientSecret: "secret",
		code:         "code",
		redirectURI:  "http://127.0.0.1:9999/callback",
		client:       server.Client(),
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if got := err.Error(); !strings.Contains(got, "500") {
		t.Errorf("error = %q, want containing status code", got)
	}
}

func TestExchangeCode_WithPKCE_IncludesCodeVerifier(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if got := r.FormValue("code_verifier"); got != "test-verifier" {
			t.Errorf("code_verifier = %q, want test-verifier", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"refresh_token":"rt"}`)) //nolint:errcheck
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "app",
		clientSecret: "secret",
		code:         "code",
		redirectURI:  "http://127.0.0.1:9999/callback",
		codeVerifier: "test-verifier",
		client:       server.Client(),
	})
	if err != nil {
		t.Fatalf("exchangeCode() error = %v", err)
	}
}

func TestExchangeCode_WithoutPKCE_OmitsCodeVerifier(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if got := r.FormValue("code_verifier"); got != "" {
			t.Errorf("code_verifier = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"refresh_token":"rt"}`)) //nolint:errcheck
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "app",
		clientSecret: "secret",
		code:         "code",
		redirectURI:  "http://127.0.0.1:9999/callback",
		client:       server.Client(),
	})
	if err != nil {
		t.Fatalf("exchangeCode() error = %v", err)
	}
}

func TestExchangeCode_ExtraParams_Included(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if got := r.FormValue("resource"); got != "https://graph.microsoft.com" {
			t.Errorf("resource = %q, want https://graph.microsoft.com", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"refresh_token":"rt"}`)) //nolint:errcheck
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	extra := url.Values{}
	extra.Set("resource", "https://graph.microsoft.com")

	_, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "app",
		clientSecret: "secret",
		code:         "code",
		redirectURI:  "http://127.0.0.1:9999/callback",
		extraParams:  extra,
		client:       server.Client(),
	})
	if err != nil {
		t.Fatalf("exchangeCode() error = %v", err)
	}
}

func TestExchangeCode_ContextCancelled_ReturnsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"refresh_token":"rt"}`)) //nolint:errcheck
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     server.URL,
		clientID:     "app",
		clientSecret: "secret",
		code:         "code",
		redirectURI:  "http://127.0.0.1:9999/callback",
		client:       server.Client(),
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
