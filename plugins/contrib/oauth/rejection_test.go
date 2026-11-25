// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/plugins/contrib"
	"github.com/cloudblue/chaperone/plugins/contrib/internal/oauthutil"
	"github.com/cloudblue/chaperone/sdk"
)

// countingTokenServer serves a different access token on every request
// ("tok-1", "tok-2", ...) and counts the requests.
func countingTokenServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":3600,"token_type":"Bearer"}`, n)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// fakeClock is a manually advanced clock for cooldown tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 18, 16, 57, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// sentWith builds the request a target received, sent with the given
// Authorization header value. It becomes the Request of the target's response.
func sentWith(ctx context.Context, t *testing.T, authorization string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.vendor.example/v1/orders", http.NoBody)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	return req
}

// rejectionProvider is what both grant types offer to these tests.
type rejectionProvider interface {
	sdk.CredentialProvider
	contrib.RejectedCredentialHandler
}

// rejectionProviders builds one provider per grant type against its own
// token server, and returns the token manager so tests can set its clock.
func rejectionProviders(t *testing.T) map[string]func() (rejectionProvider, *tokenManager, *atomic.Int32) {
	t.Helper()
	return map[string]func() (rejectionProvider, *tokenManager, *atomic.Int32){
		"client credentials": func() (rejectionProvider, *tokenManager, *atomic.Int32) {
			srv, calls := countingTokenServer(t)
			cc := NewClientCredentials(ClientCredentialsConfig{
				TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret",
			})
			return cc, cc.tm, calls
		},
		"refresh token": func() (rejectionProvider, *tokenManager, *atomic.Int32) {
			srv, calls := countingTokenServer(t)
			rt := NewRefreshToken(RefreshTokenConfig{
				TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret",
				Store: newMemoryStore("refresh-tok"),
			})
			return rt, rt.tm, calls
		},
	}
}

// authHeader fetches credentials and returns the Authorization header value.
func authHeader(ctx context.Context, t *testing.T, p sdk.CredentialProvider) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
	cred, err := p.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("GetCredentials: unexpected error: %v", err)
	}
	return cred.Headers["Authorization"]
}

func TestCredentialRejected_TargetReturns401_NextRequestFetchesNewToken(t *testing.T) {
	for name, build := range rejectionProviders(t) {
		t.Run(name, func(t *testing.T) {
			p, _, calls := build()
			ctx := context.Background()

			first := authHeader(ctx, t, p)
			if first != "Bearer tok-1" {
				t.Fatalf("first Authorization = %q, want Bearer tok-1", first)
			}
			if got := authHeader(ctx, t, p); got != first {
				t.Fatalf("cached Authorization = %q, want %q", got, first)
			}

			p.CredentialRejected(ctx, sdk.TransactionContext{}, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, first)})

			if got := authHeader(ctx, t, p); got != "Bearer tok-2" {
				t.Errorf("Authorization after 401 = %q, want Bearer tok-2 (a new token request)", got)
			}
			if got := calls.Load(); got != 2 {
				t.Errorf("token requests = %d, want 2", got)
			}
		})
	}
}

func TestCredentialRejected_ResponsesThatMustNotDiscard(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		authorization string
	}{
		{"403 Forbidden", http.StatusForbidden, "Bearer tok-1"},
		{"500 Internal Server Error", http.StatusInternalServerError, "Bearer tok-1"},
		{"200 OK", http.StatusOK, "Bearer tok-1"},
		{"401 for a token that is no longer the cached one", http.StatusUnauthorized, "Bearer some-older-token"},
		{"401 without Authorization header", http.StatusUnauthorized, ""},
	}

	for name, build := range rejectionProviders(t) {
		for _, tt := range tests {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				p, _, calls := build()
				ctx := context.Background()

				authHeader(ctx, t, p)
				p.CredentialRejected(ctx, sdk.TransactionContext{},
					&http.Response{StatusCode: tt.status, Request: sentWith(ctx, t, tt.authorization)})

				if got := authHeader(ctx, t, p); got != "Bearer tok-1" {
					t.Errorf("Authorization = %q, want the cached Bearer tok-1", got)
				}
				if got := calls.Load(); got != 1 {
					t.Errorf("token requests = %d, want 1", got)
				}
			})
		}
	}
}

func TestCredentialRejected_MalformedInput_DoesNotPanic(t *testing.T) {
	for name, build := range rejectionProviders(t) {
		t.Run(name, func(t *testing.T) {
			p, _, _ := build()
			ctx := context.Background()

			// Before any token was fetched, with a nil response, and with a
			// response that has no request.
			p.CredentialRejected(ctx, sdk.TransactionContext{}, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, "Bearer tok-1")})
			p.CredentialRejected(ctx, sdk.TransactionContext{}, nil)
			p.CredentialRejected(ctx, sdk.TransactionContext{}, &http.Response{StatusCode: http.StatusUnauthorized})

			if got := authHeader(ctx, t, p); got != "Bearer tok-1" {
				t.Errorf("Authorization = %q, want Bearer tok-1", got)
			}
		})
	}
}

// A target that answers 401 to every request, whatever the token, must not
// turn every proxied request into a token request.
func TestCredentialRejected_TargetAlwaysReturns401_TokenRequestsAreRateLimited(t *testing.T) {
	for name, build := range rejectionProviders(t) {
		t.Run(name, func(t *testing.T) {
			p, tm, calls := build()
			clock := newFakeClock()
			tm.now = clock.Now

			ctx := context.Background()
			request := func() {
				sent := authHeader(ctx, t, p)
				p.CredentialRejected(ctx, sdk.TransactionContext{},
					&http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, sent)})
			}

			for range 50 {
				request()
				clock.Advance(time.Second)
			}
			// 50 seconds elapsed: the initial request plus one forced by the first 401.
			if got := calls.Load(); got != 2 {
				t.Fatalf("token requests within the cooldown = %d, want 2", got)
			}

			// Once the cooldown has passed, one more discard is allowed.
			clock.Advance(oauthutil.RejectionCooldown)
			request()
			request()
			if got := calls.Load(); got != 3 {
				t.Errorf("token requests after the cooldown = %d, want 3", got)
			}
		})
	}
}
