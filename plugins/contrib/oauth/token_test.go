// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/sdk"
)

func TestNewTokenManager_NilLogger_LazyResolution(t *testing.T) {
	tm := newTokenManager("https://example.com/token", nil, func(_ context.Context) (*cachedToken, error) {
		return nil, nil
	})

	// logger field must be nil — no eager slog.Default() at construction.
	if tm.logger != nil {
		t.Error("logger field should be nil when not provided; lazy resolution via log()")
	}
	if tm.log() != slog.Default() {
		t.Error("log() should return slog.Default() when logger is nil")
	}
}

func TestNewTokenManager_ExplicitLogger_UsesExplicitLogger(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	tm := newTokenManager("https://example.com/token", custom, func(_ context.Context) (*cachedToken, error) {
		return nil, nil
	})

	if tm.log() != custom {
		t.Error("log() should return the explicitly provided logger")
	}
}

// hasMonotonicReading reports whether t carries a monotonic clock reading,
// which time.Time.String prints as a trailing "m=±..." field.
func hasMonotonicReading(t time.Time) bool {
	return strings.Contains(t.String(), " m=")
}

// The monotonic clock does not advance while the host is suspended, so an
// expiry that carries a monotonic reading outlives the token by the length of
// the suspension. Credentials returned by both grant types must hold a
// wall-clock-only instant, on the exchange and on a cache hit.
func TestGetCredentials_ExpiresAt_IsWallClockOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"access-tok","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer srv.Close()

	providers := map[string]sdk.CredentialProvider{
		"client credentials": NewClientCredentials(ClientCredentialsConfig{
			TokenURL:     srv.URL,
			ClientID:     "id",
			ClientSecret: "secret",
		}),
		"refresh token": NewRefreshToken(RefreshTokenConfig{
			TokenURL:     srv.URL,
			ClientID:     "id",
			ClientSecret: "secret",
			Store:        newMemoryStore("refresh-tok"),
		}),
	}

	ctx := context.Background()
	for name, provider := range providers {
		t.Run(name, func(t *testing.T) {
			for _, call := range []string{"exchange", "cache hit"} {
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
				cred, err := provider.GetCredentials(ctx, sdk.TransactionContext{}, req)
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", call, err)
				}
				if hasMonotonicReading(cred.ExpiresAt) {
					t.Errorf("%s: credential ExpiresAt = %v carries a monotonic clock reading", call, cred.ExpiresAt)
				}
			}
		})
	}
}
