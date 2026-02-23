// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

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

// memoryStore is an in-memory TokenStore for testing.
type memoryStore struct {
	mu    sync.Mutex
	token string

	loadErr   error
	saveErr   error
	saveCalls atomic.Int32
}

func newMemoryStore(initialToken string) *memoryStore {
	return &memoryStore{token: initialToken}
}

func (s *memoryStore) Load(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return "", s.loadErr
	}
	return s.token, nil
}

func (s *memoryStore) Save(_ context.Context, refreshToken string) error {
	s.saveCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.token = refreshToken
	return nil
}

func (s *memoryStore) getToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// refreshTokenHandler returns a handler that serves a valid token response
// with an optional rotated refresh token.
func refreshTokenHandler(newRefreshToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if newRefreshToken != "" {
			fmt.Fprintf(w, `{"access_token":"access-tok-abc","expires_in":3600,"token_type":"Bearer","refresh_token":%q}`, newRefreshToken)
		} else {
			fmt.Fprint(w, `{"access_token":"access-tok-abc","expires_in":3600,"token_type":"Bearer"}`)
		}
	}
}

func TestRefreshToken_ValidResponse_WithRotation_ReturnsCredentialAndSavesToken(t *testing.T) {
	srv := httptest.NewServer(refreshTokenHandler("new-refresh-tok"))
	defer srv.Close()

	store := newMemoryStore("initial-refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred, err := rt.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantHeader := "Bearer access-tok-abc"
	if got := cred.Headers["Authorization"]; got != wantHeader {
		t.Errorf("Authorization = %q, want %q", got, wantHeader)
	}

	if cred.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}

	if cred.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}

	// Verify rotated refresh token was saved
	if got := store.getToken(); got != "new-refresh-tok" {
		t.Errorf("store token = %q, want %q", got, "new-refresh-tok")
	}

	if got := store.saveCalls.Load(); got != 1 {
		t.Errorf("Store.Save called %d times, want 1", got)
	}
}

func TestRefreshToken_ValidResponse_NoRotation_DoesNotCallSave(t *testing.T) {
	srv := httptest.NewServer(refreshTokenHandler(""))
	defer srv.Close()

	store := newMemoryStore("original-refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cred.Headers["Authorization"]; got != "Bearer access-tok-abc" {
		t.Errorf("Authorization = %q, want Bearer access-tok-abc", got)
	}

	// Store.Save should NOT have been called
	if got := store.saveCalls.Load(); got != 0 {
		t.Errorf("Store.Save called %d times, want 0", got)
	}

	// Original token should be unchanged
	if got := store.getToken(); got != "original-refresh-tok" {
		t.Errorf("store token = %q, want %q", got, "original-refresh-tok")
	}
}

func TestRefreshToken_StoreLoadFailure_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(refreshTokenHandler("new-tok"))
	defer srv.Close()

	store := newMemoryStore("")
	store.loadErr = errors.New("disk read failed")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "loading refresh token") {
		t.Errorf("error = %q, want containing 'loading refresh token'", err.Error())
	}

	if !strings.Contains(err.Error(), "disk read failed") {
		t.Errorf("error = %q, want containing original error", err.Error())
	}
}

func TestRefreshToken_StoreSaveFailure_LogsErrorAndReturnsAccessToken(t *testing.T) {
	srv := httptest.NewServer(refreshTokenHandler("rotated-tok"))
	defer srv.Close()

	store := newMemoryStore("initial-tok")
	store.saveErr = errors.New("vault write failed")

	capture := &logCapture{}
	logger := slog.New(capture)

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
		Logger:       logger,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	// Should still return the access token despite Save failure
	cred, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cred.Headers["Authorization"]; got != "Bearer access-tok-abc" {
		t.Errorf("Authorization = %q, want Bearer access-tok-abc", got)
	}

	// Verify error was logged
	found := false
	for _, entry := range capture.getEntries() {
		if entry.message == "failed to save rotated refresh token" && entry.level == slog.LevelError {
			if !strings.Contains(entry.attrs["error"], "vault write failed") {
				t.Errorf("log error attr = %q, want containing 'vault write failed'", entry.attrs["error"])
			}
			found = true
		}
	}

	if !found {
		t.Error("expected error-level log entry for save failure")
	}
}

func TestRefreshToken_ErrorResponses(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantErrType error
		wantErrMsg  string
	}{
		{
			name: "401 returns ErrInvalidCredentials",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":"invalid_client"}`)
			},
			wantErrType: contrib.ErrInvalidCredentials,
		},
		{
			name: "500 returns ErrTokenEndpointUnavailable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErrType: contrib.ErrTokenEndpointUnavailable,
		},
		{
			name: "429 returns ErrTokenEndpointUnavailable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantErrType: contrib.ErrTokenEndpointUnavailable,
		},
		{
			name: "non-JSON content-type",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, "<h1>Error</h1>")
			},
			wantErrMsg: "content-type: text/html",
		},
		{
			name: "missing access_token",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"expires_in":3600}`)
			},
			wantErrMsg: "access_token",
		},
		{
			name: "missing expires_in",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"access_token":"tok"}`)
			},
			wantErrMsg: "expires_in",
		},
		{
			name: "malformed JSON body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{not valid json`)
			},
			wantErrMsg: "parsing token response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			store := newMemoryStore("refresh-tok")

			rt := NewRefreshToken(RefreshTokenConfig{
				TokenURL:     srv.URL,
				ClientID:     "id",
				ClientSecret: "secret",
				Store:        store,
			})

			ctx := context.Background()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

			_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if tt.wantErrType != nil && !errors.Is(err, tt.wantErrType) {
				t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErrType)
			}

			if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestRefreshToken_NonSuccessResponse_IncludesStatusAndContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"forbidden"}`)
	}))
	defer srv.Close()

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "403") {
		t.Errorf("error should contain status code 403, got: %q", errMsg)
	}
	if !strings.Contains(errMsg, "application/json") {
		t.Errorf("error should contain content-type, got: %q", errMsg)
	}
}

func TestRefreshToken_TokenEndpointUnreachable_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(refreshTokenHandler("tok"))
	srv.Close() // Close immediately to make endpoint unreachable

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrTokenEndpointUnavailable) {
		t.Errorf("error = %v, want errors.Is(ErrTokenEndpointUnavailable)", err)
	}
}

func TestRefreshToken_SendsRefreshTokenInForm(t *testing.T) {
	var gotBody string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryStore("my-refresh-token-123")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, "grant_type=refresh_token") {
		t.Errorf("body should contain grant_type=refresh_token, got: %q", gotBody)
	}

	if !strings.Contains(gotBody, "refresh_token=my-refresh-token-123") {
		t.Errorf("body should contain refresh_token from store, got: %q", gotBody)
	}
}

func TestRefreshToken_ExtraParams_MergedIntoBody(t *testing.T) {
	var gotBody string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Scopes:       []string{"api.read"},
		ExtraParams:  map[string]string{"resource": "https://graph.microsoft.com"},
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, "resource=") {
		t.Errorf("body should contain extra param 'resource', got: %q", gotBody)
	}
	if !strings.Contains(gotBody, "grant_type=refresh_token") {
		t.Errorf("body should contain grant_type, got: %q", gotBody)
	}
	if !strings.Contains(gotBody, "scope=api.read") {
		t.Errorf("body should contain scope, got: %q", gotBody)
	}
}

func TestRefreshToken_ExtraParams_StandardFieldsWin(t *testing.T) {
	var gotBody string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryStore("real-refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "real-id",
		ClientSecret: "real-secret",
		ExtraParams: map[string]string{
			"grant_type":    "password",        // should be ignored
			"client_id":     "injected-id",     // should be ignored
			"client_secret": "injected-secret", // should be ignored
			"scope":         "injected-scope",  // should be ignored
			"refresh_token": "injected-tok",    // should be ignored
			"resource":      "https://api.com", // should be kept
		},
		Store: store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(gotBody, "password") {
		t.Error("ExtraParams should not override grant_type")
	}
	if strings.Contains(gotBody, "injected-id") {
		t.Error("ExtraParams should not override client_id")
	}
	if strings.Contains(gotBody, "injected-secret") {
		t.Error("ExtraParams should not override client_secret")
	}
	if strings.Contains(gotBody, "injected-scope") {
		t.Error("ExtraParams should not override scope")
	}
	if strings.Contains(gotBody, "injected-tok") {
		t.Error("ExtraParams should not override refresh_token")
	}
	if !strings.Contains(gotBody, "resource=") {
		t.Error("non-standard ExtraParams should be included")
	}
}

func TestRefreshToken_AuthModeBasic_SendsBasicHeader(t *testing.T) {
	var (
		gotAuthHeader string
		gotBody       string
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "basic-id",
		ClientSecret: "basic-secret",
		AuthMode:     AuthModeBasic,
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(gotAuthHeader, "Basic ") {
		t.Errorf("expected Basic auth header, got: %q", gotAuthHeader)
	}

	if strings.Contains(gotBody, "client_id") {
		t.Error("AuthModeBasic should not include client_id in form body")
	}
	if strings.Contains(gotBody, "client_secret") {
		t.Error("AuthModeBasic should not include client_secret in form body")
	}
}

func TestRefreshToken_ExpiryMargin_SubtractedFromTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	}))
	defer srv.Close()

	store := newMemoryStore("refresh-tok")
	margin := 5 * time.Minute

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
		ExpiryMargin: margin,
	})

	before := time.Now()
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Token TTL should be approximately 3600s - 300s = 3300s
	expectedMin := before.Add(3600*time.Second - margin - 2*time.Second)
	expectedMax := time.Now().Add(3600*time.Second - margin + 2*time.Second)

	if cred.ExpiresAt.Before(expectedMin) || cred.ExpiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt = %v, want between %v and %v",
			cred.ExpiresAt, expectedMin, expectedMax)
	}
}

func TestRefreshToken_ExpiresIn_LessThanMargin_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":30}`)
	}))
	defer srv.Close()

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
		ExpiryMargin: 60 * time.Second, // 60s margin > 30s expires_in
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrTokenExpiredOnArrival) {
		t.Errorf("error = %v, want errors.Is(ErrTokenExpiredOnArrival)", err)
	}
}

func TestRefreshToken_CacheHit_SingleHTTPCall(t *testing.T) {
	var callCount atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"cached-tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred1, err := rt.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	cred2, err := rt.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := callCount.Load(); got != 1 {
		t.Errorf("expected 1 HTTP call, got %d", got)
	}

	if cred1.Headers["Authorization"] != cred2.Headers["Authorization"] {
		t.Error("both calls should return the same token")
	}
}

func TestRefreshToken_Singleflight_DeduplicatesConcurrentRequests(t *testing.T) {
	var callCount atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		time.Sleep(100 * time.Millisecond) // Hold to ensure overlap
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"shared-tok","expires_in":3600,"refresh_token":"rotated"}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Store:        store,
	})

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := context.Background()
			tx := sdk.TransactionContext{}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
			_, errs[idx] = rt.GetCredentials(ctx, tx, req)
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

func TestRefreshToken_CustomHTTPClient(t *testing.T) {
	var gotCustomHeader string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustomHeader = r.Header.Get("X-Custom-Test")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	customClient := &http.Client{
		Transport: &headerInjectTransport{
			key:        "X-Custom-Test",
			value:      "custom-value",
			underlying: http.DefaultTransport,
		},
	}

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		HTTPClient:   customClient,
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotCustomHeader != "custom-value" {
		t.Errorf("custom HTTP client not used: X-Custom-Test = %q, want %q",
			gotCustomHeader, "custom-value")
	}
}

func TestRefreshToken_NonSuccessResponse_LogsBodyPrefix(t *testing.T) {
	responseBody := `{"error":"access_denied","error_description":"insufficient scope"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, responseBody)
	}))
	defer srv.Close()

	capture := &logCapture{}
	logger := slog.New(capture)

	store := newMemoryStore("refresh-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Logger:       logger,
		Store:        store,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := rt.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	found := false
	for _, entry := range capture.getEntries() {
		if entry.message != "token endpoint error response" {
			continue
		}
		if entry.attrs["body_prefix"] == "" {
			t.Error("body_prefix attribute should be present")
		}
		if !strings.Contains(entry.attrs["body_prefix"], "access_denied") {
			t.Errorf("body_prefix = %q, want to contain response body",
				entry.attrs["body_prefix"])
		}
		found = true
	}

	if !found {
		t.Error("expected 'token endpoint error response' log entry with body_prefix")
	}
}

func TestRefreshToken_Compliance(t *testing.T) {
	srv := httptest.NewServer(refreshTokenHandler("rotated-tok"))
	defer srv.Close()

	store := newMemoryStore("initial-tok")

	rt := NewRefreshToken(RefreshTokenConfig{
		TokenURL:     srv.URL,
		ClientID:     "compliance-id",
		ClientSecret: "compliance-secret",
		Store:        store,
	})

	plugin := contrib.AsPlugin(rt)
	compliance.VerifyContract(t, plugin)
}
