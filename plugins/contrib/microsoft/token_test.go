// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package microsoft

import (
	"context"
	"errors"
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
	"github.com/cloudblue/chaperone/sdk"
	"github.com/cloudblue/chaperone/sdk/compliance"
)

// --- Test helpers ---

// memoryTokenStore is a keyed in-memory TokenStore for testing.
type memoryTokenStore struct {
	mu     sync.Mutex
	tokens map[string]string // key: "tenantID|resource"

	loadErr   error
	saveErr   error
	saveCalls atomic.Int32
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{tokens: make(map[string]string)}
}

func (s *memoryTokenStore) set(tenantID, resource, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[tenantID+"|"+resource] = token
}

func (s *memoryTokenStore) get(tenantID, resource string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens[tenantID+"|"+resource]
}

func (s *memoryTokenStore) Load(_ context.Context, tenantID, resource string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return "", s.loadErr
	}
	tok, ok := s.tokens[tenantID+"|"+resource]
	if !ok {
		return "", fmt.Errorf("no token for tenant %s, resource %s: %w",
			tenantID, resource, contrib.ErrTenantNotFound)
	}
	return tok, nil
}

func (s *memoryTokenStore) Save(_ context.Context, tenantID, resource, refreshToken string) error {
	s.saveCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.tokens[tenantID+"|"+resource] = refreshToken
	return nil
}

// tokenHandler returns a handler that serves a valid v1 token response
// with an optional rotated refresh token.
func tokenHandler(newRefreshToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if newRefreshToken != "" {
			fmt.Fprintf(w, `{"access_token":"access-tok","expires_in":3600,"token_type":"Bearer","refresh_token":%q}`,
				newRefreshToken)
		} else {
			fmt.Fprint(w, `{"access_token":"access-tok","expires_in":3600,"token_type":"Bearer"}`)
		}
	}
}

func makeTx(tenantID, resource string) sdk.TransactionContext {
	return sdk.TransactionContext{
		Data: map[string]any{
			"TenantID": tenantID,
			"Resource": resource,
		},
		VendorID: "microsoft-test",
	}
}

func makeReq(ctx context.Context) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.microsoft.com/v1.0/me", http.NoBody)
	return req
}

type logCapture struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	level   slog.Level
	message string
	attrs   map[string]string
}

func (lc *logCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (lc *logCapture) Handle(_ context.Context, r slog.Record) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	entry := logEntry{
		level:   r.Level,
		message: r.Message,
		attrs:   make(map[string]string),
	}
	r.Attrs(func(a slog.Attr) bool {
		entry.attrs[a.Key] = a.Value.String()
		return true
	})

	lc.entries = append(lc.entries, entry)
	return nil
}

func (lc *logCapture) WithAttrs(attrs []slog.Attr) slog.Handler {
	return lc
}

func (lc *logCapture) WithGroup(_ string) slog.Handler {
	return lc
}

func (lc *logCapture) getEntries() []logEntry {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	cp := make([]logEntry, len(lc.entries))
	copy(cp, lc.entries)
	return cp
}

// --- Tests ---

func TestGetCredentials_MissingTenantID_ReturnsErrMissingContextData(t *testing.T) {
	store := newMemoryTokenStore()

	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{
		Data: map[string]any{
			"Resource": "https://graph.microsoft.com",
		},
	}

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrMissingContextData) {
		t.Errorf("error = %v, want errors.Is(ErrMissingContextData)", err)
	}

	if !strings.Contains(err.Error(), "TenantID") {
		t.Errorf("error = %q, want containing 'TenantID'", err.Error())
	}
}

func TestGetCredentials_MissingResource_ReturnsErrMissingContextData(t *testing.T) {
	store := newMemoryTokenStore()

	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{
		Data: map[string]any{
			"TenantID": "contoso.onmicrosoft.com",
		},
	}

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrMissingContextData) {
		t.Errorf("error = %v, want errors.Is(ErrMissingContextData)", err)
	}

	if !strings.Contains(err.Error(), "Resource") {
		t.Errorf("error = %q, want containing 'Resource'", err.Error())
	}
}

func TestGetCredentials_EmptyTenantID_ReturnsErrInvalidContextData(t *testing.T) {
	store := newMemoryTokenStore()

	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{
		Data: map[string]any{
			"TenantID": "",
			"Resource": "https://graph.microsoft.com",
		},
	}

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrInvalidContextData) {
		t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
	}

	if !strings.Contains(err.Error(), "TenantID") {
		t.Errorf("error = %q, want containing 'TenantID'", err.Error())
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %q, want containing 'empty'", err.Error())
	}
}

func TestGetCredentials_EmptyResource_ReturnsErrInvalidContextData(t *testing.T) {
	store := newMemoryTokenStore()

	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{
		Data: map[string]any{
			"TenantID": "contoso.onmicrosoft.com",
			"Resource": "",
		},
	}

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrInvalidContextData) {
		t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
	}

	if !strings.Contains(err.Error(), "Resource") {
		t.Errorf("error = %q, want containing 'Resource'", err.Error())
	}
}

func TestGetCredentials_TenantIDWrongType_ReturnsErrInvalidContextData(t *testing.T) {
	store := newMemoryTokenStore()

	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{
		Data: map[string]any{
			"TenantID": float64(12345), // wrong type — JSON numbers unmarshal as float64
			"Resource": "https://graph.microsoft.com",
		},
	}

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrInvalidContextData) {
		t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
	}

	if !strings.Contains(err.Error(), "float64") {
		t.Errorf("error = %q, want containing actual type 'float64'", err.Error())
	}
}

func TestGetCredentials_MaliciousTenantID_ReturnsErrInvalidContextData(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
	}{
		{"path traversal", "../../admin"},
		{"query injection", "contoso?foo=bar"},
		{"fragment injection", "contoso#section"},
		{"slash in value", "contoso/evil"},
		{"backslash", "contoso\\evil"},
		{"space", "contoso evil"},
		{"starts with dot", ".hidden"},
		{"starts with hyphen", "-invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryTokenStore()

			src := NewRefreshTokenSource(Config{
				ClientID:     "id",
				ClientSecret: "secret",
				Store:        store,
			})

			ctx := context.Background()
			tx := sdk.TransactionContext{
				Data: map[string]any{
					"TenantID": tt.tenantID,
					"Resource": "https://graph.microsoft.com",
				},
			}

			_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
			if err == nil {
				t.Fatalf("expected error for tenantID %q", tt.tenantID)
			}

			if !errors.Is(err, contrib.ErrInvalidContextData) {
				t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
			}
		})
	}
}

func TestGetCredentials_ValidTenantIDFormats(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
	}{
		{"GUID", "12345678-abcd-1234-abcd-1234567890ab"},
		{"domain name", "contoso.onmicrosoft.com"},
		{"simple name", "common"},
		{"organizations", "organizations"},
		{"consumers", "consumers"},
		{"alphanumeric with hyphens", "my-tenant-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
			})

			srv := httptest.NewServer(handler)
			defer srv.Close()

			store := newMemoryTokenStore()
			store.set(tt.tenantID, "https://graph.microsoft.com", "refresh-tok")

			src := NewRefreshTokenSource(Config{
				TokenEndpoint: srv.URL,
				ClientID:      "id",
				ClientSecret:  "secret",
				Store:         store,
			})

			ctx := context.Background()
			tx := makeTx(tt.tenantID, "https://graph.microsoft.com")

			_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
			if err != nil {
				t.Fatalf("unexpected error for valid tenantID %q: %v", tt.tenantID, err)
			}
		})
	}
}

func TestGetCredentials_V1TokenEndpointURLConstruction(t *testing.T) {
	var gotURL string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("contoso.onmicrosoft.com", "https://graph.microsoft.com", "refresh-tok")

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	ctx := context.Background()
	tx := makeTx("contoso.onmicrosoft.com", "https://graph.microsoft.com")

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPath := "/contoso.onmicrosoft.com/oauth2/token"
	if gotURL != wantPath {
		t.Errorf("token endpoint path = %q, want %q", gotURL, wantPath)
	}
}

func TestGetCredentials_ResourceParameterInRequestBody(t *testing.T) {
	var gotBody string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("tenant-1", "https://graph.microsoft.com", "refresh-tok")

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	ctx := context.Background()
	tx := makeTx("tenant-1", "https://graph.microsoft.com")

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, "resource=") {
		t.Errorf("body should contain 'resource' param, got: %q", gotBody)
	}

	if !strings.Contains(gotBody, "grant_type=refresh_token") {
		t.Errorf("body should contain grant_type=refresh_token, got: %q", gotBody)
	}
}

func TestGetCredentials_CustomTokenEndpoint_GovernmentCloud(t *testing.T) {
	var gotURL string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"gov-tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("gov-tenant", "https://graph.microsoft.us", "refresh-tok")

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL, // simulates government cloud endpoint
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	ctx := context.Background()
	tx := makeTx("gov-tenant", "https://graph.microsoft.us")

	cred, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPath := "/gov-tenant/oauth2/token"
	if gotURL != wantPath {
		t.Errorf("token endpoint path = %q, want %q", gotURL, wantPath)
	}

	if got := cred.Headers["Authorization"]; got != "Bearer gov-tok" {
		t.Errorf("Authorization = %q, want Bearer gov-tok", got)
	}
}

func TestGetCredentials_PoolAtMaxCapacity_EvictsLRU(t *testing.T) {
	srv := httptest.NewServer(tokenHandler(""))
	defer srv.Close()

	store := newMemoryTokenStore()
	// Pre-populate 3 tenants
	for i := range 3 {
		store.set(fmt.Sprintf("tenant-%d", i), "https://graph.microsoft.com",
			fmt.Sprintf("refresh-tok-%d", i))
	}

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
		MaxPoolSize:   2, // small pool for testing
	})

	ctx := context.Background()

	// Fill pool with tenant-0 and tenant-1
	_, err := src.GetCredentials(ctx,
		makeTx("tenant-0", "https://graph.microsoft.com"), makeReq(ctx))
	if err != nil {
		t.Fatalf("tenant-0: %v", err)
	}

	_, err = src.GetCredentials(ctx,
		makeTx("tenant-1", "https://graph.microsoft.com"), makeReq(ctx))
	if err != nil {
		t.Fatalf("tenant-1: %v", err)
	}

	// Pool is full (2/2). Adding tenant-2 should evict tenant-0 (LRU).
	_, err = src.GetCredentials(ctx,
		makeTx("tenant-2", "https://graph.microsoft.com"), makeReq(ctx))
	if err != nil {
		t.Fatalf("tenant-2: %v", err)
	}

	src.mu.Lock()
	poolSize := len(src.pool)
	_, hasTenant0 := src.pool[poolKey{tenantID: "tenant-0", resource: "https://graph.microsoft.com"}]
	_, hasTenant1 := src.pool[poolKey{tenantID: "tenant-1", resource: "https://graph.microsoft.com"}]
	_, hasTenant2 := src.pool[poolKey{tenantID: "tenant-2", resource: "https://graph.microsoft.com"}]
	src.mu.Unlock()

	if poolSize != 2 {
		t.Errorf("pool size = %d, want 2", poolSize)
	}

	if hasTenant0 {
		t.Error("tenant-0 should have been evicted (LRU)")
	}

	if !hasTenant1 {
		t.Error("tenant-1 should still be in pool")
	}

	if !hasTenant2 {
		t.Error("tenant-2 should be in pool")
	}
}

func TestGetCredentials_DifferentTenants_SeparateInstances(t *testing.T) {
	var mu sync.Mutex
	tenantsSeen := make(map[string]bool)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract tenant from URL path: /{tenantID}/oauth2/token
		parts := strings.SplitN(r.URL.Path, "/", 3)
		if len(parts) >= 2 {
			mu.Lock()
			tenantsSeen[parts[1]] = true
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("tenant-a", "https://graph.microsoft.com", "tok-a")
	store.set("tenant-b", "https://graph.microsoft.com", "tok-b")

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	ctx := context.Background()

	_, err := src.GetCredentials(ctx,
		makeTx("tenant-a", "https://graph.microsoft.com"), makeReq(ctx))
	if err != nil {
		t.Fatalf("tenant-a: %v", err)
	}

	_, err = src.GetCredentials(ctx,
		makeTx("tenant-b", "https://graph.microsoft.com"), makeReq(ctx))
	if err != nil {
		t.Fatalf("tenant-b: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !tenantsSeen["tenant-a"] {
		t.Error("tenant-a should have made a token request")
	}
	if !tenantsSeen["tenant-b"] {
		t.Error("tenant-b should have made a token request")
	}
}

func TestGetCredentials_Singleflight_SameTenantResource(t *testing.T) {
	var callCount atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"shared-tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("tenant-sf", "https://graph.microsoft.com", "refresh-tok")

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := context.Background()
			tx := makeTx("tenant-sf", "https://graph.microsoft.com")
			_, errs[idx] = src.GetCredentials(ctx, tx, makeReq(ctx))
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	if got := callCount.Load(); got != 1 {
		t.Errorf("expected 1 HTTP call (singleflight), got %d", got)
	}
}

func TestKeyedStoreAdapter_BridgesKeyedToKeyless(t *testing.T) {
	store := newMemoryTokenStore()
	store.set("tenant-x", "resource-y", "original-tok")

	adapter := &keyedStoreAdapter{
		store:    store,
		tenantID: "tenant-x",
		resource: "resource-y",
	}

	ctx := context.Background()

	// Load should delegate with bound keys
	tok, err := adapter.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tok != "original-tok" {
		t.Errorf("Load = %q, want %q", tok, "original-tok")
	}

	// Save should delegate with bound keys
	err = adapter.Save(ctx, "rotated-tok")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := store.get("tenant-x", "resource-y"); got != "rotated-tok" {
		t.Errorf("store token = %q, want %q", got, "rotated-tok")
	}
}

func TestGetCredentials_EndToEnd_RefreshTokenRotation(t *testing.T) {
	srv := httptest.NewServer(tokenHandler("new-refresh-tok"))
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("contoso", "https://graph.microsoft.com", "initial-refresh-tok")

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	ctx := context.Background()
	tx := makeTx("contoso", "https://graph.microsoft.com")

	cred, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cred.Headers["Authorization"]; got != "Bearer access-tok" {
		t.Errorf("Authorization = %q, want Bearer access-tok", got)
	}

	if cred.ExpiresAt.IsZero() || cred.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}

	// Verify rotated refresh token was saved back to the keyed store
	if got := store.get("contoso", "https://graph.microsoft.com"); got != "new-refresh-tok" {
		t.Errorf("store token = %q, want %q", got, "new-refresh-tok")
	}
}

func TestGetCredentials_TenantNotFound_ReturnsErrTenantNotFound(t *testing.T) {
	srv := httptest.NewServer(tokenHandler(""))
	defer srv.Close()

	store := newMemoryTokenStore() // empty — no tenants

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	ctx := context.Background()
	tx := makeTx("unknown-tenant", "https://graph.microsoft.com")

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrTenantNotFound) {
		t.Errorf("error = %v, want errors.Is(ErrTenantNotFound)", err)
	}
}

func TestGetCredentials_StoreSaveFailure_LogsErrorAndReturnsToken(t *testing.T) {
	srv := httptest.NewServer(tokenHandler("rotated-tok"))
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("tenant-s", "https://graph.microsoft.com", "initial-tok")
	store.saveErr = errors.New("vault write failed")

	capture := &logCapture{}
	logger := slog.New(capture)

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
		Logger:        logger,
	})

	ctx := context.Background()
	tx := makeTx("tenant-s", "https://graph.microsoft.com")

	// Should still return the access token despite Save failure
	cred, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cred.Headers["Authorization"]; got != "Bearer access-tok" {
		t.Errorf("Authorization = %q, want Bearer access-tok", got)
	}

	// Verify error was logged
	found := false
	for _, entry := range capture.getEntries() {
		if entry.message == "failed to save rotated refresh token" && entry.level == slog.LevelError {
			if !strings.Contains(entry.attrs["error"], "vault write failed") {
				t.Errorf("log error attr = %q, want containing 'vault write failed'",
					entry.attrs["error"])
			}
			found = true
		}
	}

	if !found {
		t.Error("expected error-level log entry for save failure")
	}
}

func TestGetCredentials_ConcurrentSafety(t *testing.T) {
	srv := httptest.NewServer(tokenHandler("rotated"))
	defer srv.Close()

	store := newMemoryTokenStore()
	for i := range 5 {
		store.set(fmt.Sprintf("tenant-%d", i), "https://graph.microsoft.com",
			fmt.Sprintf("tok-%d", i))
	}

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "id",
		ClientSecret:  "secret",
		Store:         store,
	})

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tenantID := fmt.Sprintf("tenant-%d", idx%5)
			ctx := context.Background()
			tx := makeTx(tenantID, "https://graph.microsoft.com")
			_, errs[idx] = src.GetCredentials(ctx, tx, makeReq(ctx))
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

func TestGetCredentials_NilData_ReturnsErrMissingContextData(t *testing.T) {
	store := newMemoryTokenStore()

	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{Data: nil}

	_, err := src.GetCredentials(ctx, tx, makeReq(ctx))
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrMissingContextData) {
		t.Errorf("error = %v, want errors.Is(ErrMissingContextData)", err)
	}
}

func TestGetCredentials_DefaultTokenEndpoint(t *testing.T) {
	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        newMemoryTokenStore(),
	})

	if src.tokenEndpoint != "https://login.microsoftonline.com" {
		t.Errorf("tokenEndpoint = %q, want default", src.tokenEndpoint)
	}
}

func TestGetCredentials_DefaultMaxPoolSize(t *testing.T) {
	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        newMemoryTokenStore(),
	})

	if src.maxPoolSize != 10_000 {
		t.Errorf("maxPoolSize = %d, want 10000", src.maxPoolSize)
	}
}

func TestGetCredentials_DefaultExpiryMargin(t *testing.T) {
	src := NewRefreshTokenSource(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        newMemoryTokenStore(),
	})

	if src.expiryMargin != 5*time.Minute {
		t.Errorf("expiryMargin = %v, want 5m", src.expiryMargin)
	}
}

func TestRefreshTokenSource_Compliance(t *testing.T) {
	srv := httptest.NewServer(tokenHandler("rotated-tok"))
	defer srv.Close()

	store := newMemoryTokenStore()
	store.set("compliance-tenant", "https://graph.microsoft.com", "initial-tok")

	src := NewRefreshTokenSource(Config{
		TokenEndpoint: srv.URL,
		ClientID:      "compliance-id",
		ClientSecret:  "compliance-secret",
		Store:         store,
	})

	// The compliance test calls GetCredentials with a default TransactionContext.
	// We need to ensure the tx.Data fields are present. Since the compliance kit
	// uses its own TransactionContext, we wrap the source to inject the required
	// context data.
	wrapper := &complianceWrapper{
		src:      src,
		tenantID: "compliance-tenant",
		resource: "https://graph.microsoft.com",
	}

	plugin := contrib.AsPlugin(wrapper)
	compliance.VerifyContract(t, plugin)
}

// complianceWrapper injects required context data fields for the compliance
// test, which uses a default TransactionContext without Microsoft-specific
// fields.
type complianceWrapper struct {
	src      *RefreshTokenSource
	tenantID string
	resource string
}

func (w *complianceWrapper) GetCredentials(
	ctx context.Context,
	tx sdk.TransactionContext,
	req *http.Request,
) (*sdk.Credential, error) {
	if tx.Data == nil {
		tx.Data = make(map[string]any)
	}
	tx.Data["TenantID"] = w.tenantID
	tx.Data["Resource"] = w.resource
	return w.src.GetCredentials(ctx, tx, req)
}
