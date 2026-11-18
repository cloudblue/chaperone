// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package microsoft

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/plugins/contrib"
	"github.com/cloudblue/chaperone/plugins/contrib/internal/oauthutil"
	"github.com/cloudblue/chaperone/sdk"
)

const graphResource = "https://graph.microsoft.com"

// countingTokenServer serves a different access token on every exchange
// ("tok-1", "tok-2", ...) and counts the exchanges.
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
		"https://api.partnercenter.microsoft.com/v1/customers", http.NoBody)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	return req
}

func newRejectionSource(t *testing.T, tenants ...string) (*RefreshTokenSource, *atomic.Int32) {
	t.Helper()
	srv, calls := countingTokenServer(t)
	store := newMemoryTokenStore()
	for _, tenant := range tenants {
		store.set(tenant, "refresh-tok")
	}
	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return src, calls
}

// authHeader fetches credentials and returns the Authorization header value.
func authHeader(ctx context.Context, t *testing.T, src *RefreshTokenSource, tx sdk.TransactionContext) string {
	t.Helper()
	cred, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("GetCredentials: unexpected error: %v", err)
	}
	return cred.Headers["Authorization"]
}

func TestCredentialRejected_TargetReturns401_NextRequestExchangesNewToken(t *testing.T) {
	src, calls := newRejectionSource(t, "contoso")
	ctx := context.Background()
	tx := makeTx("contoso", graphResource)

	first := authHeader(ctx, t, src, tx)
	if first != "Bearer tok-1" {
		t.Fatalf("first Authorization = %q, want Bearer tok-1", first)
	}

	// Without the 401, the token is served from the cache.
	if got := authHeader(ctx, t, src, tx); got != first {
		t.Fatalf("cached Authorization = %q, want %q", got, first)
	}

	// The target rejects the request that carried the cached token.
	src.CredentialRejected(ctx, tx, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, first)})

	if got := authHeader(ctx, t, src, tx); got != "Bearer tok-2" {
		t.Errorf("Authorization after 401 = %q, want Bearer tok-2 (a new exchange)", got)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("token exchanges = %d, want 2", got)
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
		{"401 with a non-bearer scheme", http.StatusUnauthorized, "Basic dG9rLTE="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, calls := newRejectionSource(t, "contoso")
			ctx := context.Background()
			tx := makeTx("contoso", graphResource)

			authHeader(ctx, t, src, tx)
			src.CredentialRejected(ctx, tx, &http.Response{StatusCode: tt.status, Request: sentWith(ctx, t, tt.authorization)})

			if got := authHeader(ctx, t, src, tx); got != "Bearer tok-1" {
				t.Errorf("Authorization = %q, want the cached Bearer tok-1", got)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("token exchanges = %d, want 1", got)
			}
		})
	}
}

func TestCredentialRejected_MalformedInput_DoesNotPanic(t *testing.T) {
	src, _ := newRejectionSource(t, "contoso")
	ctx := context.Background()
	tx := makeTx("contoso", graphResource)
	authHeader(ctx, t, src, tx)

	// nil response, response without request, transaction without keys,
	// tenant that has no pool entry.
	src.CredentialRejected(ctx, tx, nil)
	src.CredentialRejected(ctx, tx, &http.Response{StatusCode: http.StatusUnauthorized})
	src.CredentialRejected(ctx, sdk.TransactionContext{}, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, "Bearer tok-1")})
	src.CredentialRejected(ctx, makeTx("unknown-tenant", graphResource),
		&http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, "Bearer tok-1")})

	if got := authHeader(ctx, t, src, tx); got != "Bearer tok-1" {
		t.Errorf("Authorization = %q, want the cached Bearer tok-1", got)
	}
}

// A target that answers 401 to every request, whatever the token, must not
// turn every proxied request into a token exchange.
func TestCredentialRejected_TargetAlwaysReturns401_ExchangesAreRateLimited(t *testing.T) {
	src, calls := newRejectionSource(t, "contoso")
	clock := newFakeClock()
	src.now = clock.Now

	ctx := context.Background()
	tx := makeTx("contoso", graphResource)

	request := func() {
		sent := authHeader(ctx, t, src, tx)
		src.CredentialRejected(ctx, tx, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, sent)})
	}

	for range 50 {
		request()
		clock.Advance(time.Second)
	}
	// 50 seconds elapsed: the initial exchange plus one forced by the first 401.
	if got := calls.Load(); got != 2 {
		t.Fatalf("token exchanges within the cooldown = %d, want 2", got)
	}

	// Once the cooldown has passed, one more discard is allowed.
	clock.Advance(oauthutil.RejectionCooldown)
	request()
	request()
	if got := calls.Load(); got != 3 {
		t.Errorf("token exchanges after the cooldown = %d, want 3", got)
	}
}

// The limit is per tenant and resource: a 401 on one pair neither discards
// nor consumes the allowance of another.
func TestCredentialRejected_IsScopedToTenantAndResource(t *testing.T) {
	src, calls := newRejectionSource(t, "contoso", "fabrikam")
	clock := newFakeClock()
	src.now = clock.Now
	ctx := context.Background()

	contosoGraph := makeTx("contoso", graphResource)
	contosoMgmt := makeTx("contoso", "https://management.azure.com")
	fabrikamGraph := makeTx("fabrikam", graphResource)

	sentContosoGraph := authHeader(ctx, t, src, contosoGraph)
	sentContosoMgmt := authHeader(ctx, t, src, contosoMgmt)
	sentFabrikamGraph := authHeader(ctx, t, src, fabrikamGraph)
	if got := calls.Load(); got != 3 {
		t.Fatalf("token exchanges = %d, want 3", got)
	}

	src.CredentialRejected(ctx, contosoGraph, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, sentContosoGraph)})

	// The other two pairs are still cached.
	if got := authHeader(ctx, t, src, contosoMgmt); got != sentContosoMgmt {
		t.Errorf("contoso/management Authorization = %q, want cached %q", got, sentContosoMgmt)
	}
	if got := authHeader(ctx, t, src, fabrikamGraph); got != sentFabrikamGraph {
		t.Errorf("fabrikam/graph Authorization = %q, want cached %q", got, sentFabrikamGraph)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("token exchanges = %d, want 3 (only contoso/graph was discarded, not yet re-fetched)", got)
	}

	// And each keeps its own allowance within the same minute.
	src.CredentialRejected(ctx, contosoMgmt, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, sentContosoMgmt)})
	src.CredentialRejected(ctx, fabrikamGraph, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, sentFabrikamGraph)})
	authHeader(ctx, t, src, contosoGraph)
	authHeader(ctx, t, src, contosoMgmt)
	authHeader(ctx, t, src, fabrikamGraph)
	if got := calls.Load(); got != 6 {
		t.Errorf("token exchanges = %d, want 6 (one forced exchange per pair)", got)
	}
}

func TestCredentialRejected_TenantFromKeyResolver(t *testing.T) {
	srv, calls := countingTokenServer(t)
	store := newMemoryTokenStore()
	store.set("contoso", "refresh-tok")
	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
		KeyResolver:   &staticResolver{key: "contoso"},
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{Data: map[string]any{"Resource": graphResource}}

	sent := authHeader(ctx, t, src, tx)
	src.CredentialRejected(ctx, tx, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, sent)})

	if got := authHeader(ctx, t, src, tx); got != "Bearer tok-2" {
		t.Errorf("Authorization after 401 = %q, want Bearer tok-2", got)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("token exchanges = %d, want 2", got)
	}
}

func TestCredentialRejected_LogsDiscardAtWarnWithoutTheToken(t *testing.T) {
	srv, _ := countingTokenServer(t)
	store := newMemoryTokenStore()
	store.set("contoso", "refresh-tok")
	capture := &logCapture{}
	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
		Logger:        slog.New(capture),
	})

	ctx := context.Background()
	tx := makeTx("contoso", graphResource)
	sent := authHeader(ctx, t, src, tx)
	src.CredentialRejected(ctx, tx, &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, sent)})

	var found bool
	for _, e := range capture.getEntries() {
		if strings.Contains(e.message, "tok-1") {
			t.Errorf("log message leaks the access token: %q", e.message)
		}
		for k, v := range e.attrs {
			if strings.Contains(v, "tok-1") {
				t.Errorf("log attribute %s leaks the access token: %q", k, v)
			}
		}
		if e.level == slog.LevelWarn && strings.Contains(e.message, "target rejected the cached access token") {
			found = true
			if e.attrs["tenant_id"] != "contoso" || e.attrs["resource"] != graphResource {
				t.Errorf("warn attrs = %v, want tenant_id=contoso resource=%s", e.attrs, graphResource)
			}
		}
	}
	if !found {
		t.Error("expected a WARN log entry for the discarded token")
	}
}

// The Mux forwards a 401 to the provider that served the request.
func TestCredentialRejected_ThroughMux(t *testing.T) {
	src, calls := newRejectionSource(t, "contoso")

	mux := contrib.NewMux()
	mux.Handle(contrib.Route{VendorID: "microsoft-*"}, src)

	ctx := context.Background()
	tx := makeTx("contoso", graphResource)

	cred, err := mux.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}

	resp := &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith(ctx, t, cred.Headers["Authorization"])}
	if _, err = mux.ModifyResponse(ctx, tx, resp); err != nil {
		t.Fatalf("ModifyResponse: %v", err)
	}

	cred, err = mux.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("GetCredentials after 401: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer tok-2" {
		t.Errorf("Authorization after 401 = %q, want Bearer tok-2", got)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("token exchanges = %d, want 2", got)
	}
}
