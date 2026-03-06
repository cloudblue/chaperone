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

// validTokenHandler returns a handler that serves a valid token response.
func validTokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"test-token-abc","expires_in":3600,"token_type":"Bearer"}`)
	}
}

func TestGetCredentials_StringExpiresIn_ReturnsCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Some providers (e.g., Microsoft) return expires_in as a string.
		fmt.Fprint(w, `{"access_token":"test-token-abc","expires_in":"3600","token_type":"Bearer"}`)
	}))
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "test-id",
		ClientSecret: "test-secret",
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred, err := cc.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cred.Headers["Authorization"]; got != "Bearer test-token-abc" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token-abc")
	}

	if cred.ExpiresAt.IsZero() || cred.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}
}

func TestGetCredentials_ValidResponse_ReturnsCredential(t *testing.T) {
	srv := httptest.NewServer(validTokenHandler())
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Scopes:       []string{"api.read", "api.write"},
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred, err := cc.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantHeader := "Bearer test-token-abc"
	if got := cred.Headers["Authorization"]; got != wantHeader {
		t.Errorf("Authorization = %q, want %q", got, wantHeader)
	}

	if cred.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}

	if cred.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}
}

func TestGetCredentials_ErrorResponses(t *testing.T) {
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
			name: "403 includes status code and content-type",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"access_denied"}`)
			},
			wantErrMsg: "403",
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
			name: "malformed JSON body with valid content-type",
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

			cc := NewClientCredentials(ClientCredentialsConfig{
				TokenURL:     srv.URL,
				ClientID:     "test-id",
				ClientSecret: "test-secret",
			})

			ctx := context.Background()
			tx := sdk.TransactionContext{}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

			_, err := cc.GetCredentials(ctx, tx, req)
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

func TestGetCredentials_NonSuccessResponse_IncludesStatusAndContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"forbidden"}`)
	}))
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
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

func TestGetCredentials_TokenEndpointUnreachable_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(validTokenHandler())
	srv.Close() // Close immediately to make endpoint unreachable

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrTokenEndpointUnavailable) {
		t.Errorf("error = %v, want errors.Is(ErrTokenEndpointUnavailable)", err)
	}
}

func TestGetCredentials_CacheHit_SingleHTTPCall(t *testing.T) {
	var callCount atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"cached-tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred1, err := cc.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	cred2, err := cc.GetCredentials(ctx, tx, req)
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

func TestGetCredentials_ExpiredToken_Refetches(t *testing.T) {
	var callCount atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":1}`, n)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		ExpiryMargin: 100 * time.Millisecond,
	})

	ctx := context.Background()
	tx := sdk.TransactionContext{}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred1, err := cc.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Token TTL is 1s - 100ms = 900ms. Wait for expiry.
	time.Sleep(1 * time.Second)

	cred2, err := cc.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := callCount.Load(); got != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", got)
	}

	if cred1.Headers["Authorization"] == cred2.Headers["Authorization"] {
		t.Error("expected different tokens after expiry")
	}
}

func TestGetCredentials_Singleflight_DeduplicatesConcurrentRequests(t *testing.T) {
	var callCount atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		time.Sleep(100 * time.Millisecond) // Hold to ensure overlap
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"shared-tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
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
			_, errs[idx] = cc.GetCredentials(ctx, tx, req)
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

func TestGetCredentials_ExtraParams_MergedIntoBody(t *testing.T) {
	var gotBody string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Scopes:       []string{"api.read"},
		ExtraParams:  map[string]string{"audience": "https://api.vendor.com"},
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, "audience=https") {
		t.Errorf("body should contain extra param 'audience', got: %q", gotBody)
	}
	if !strings.Contains(gotBody, "grant_type=client_credentials") {
		t.Errorf("body should contain grant_type, got: %q", gotBody)
	}
	if !strings.Contains(gotBody, "scope=api.read") {
		t.Errorf("body should contain scope, got: %q", gotBody)
	}
}

func TestGetCredentials_ExtraParams_StandardFieldsWin(t *testing.T) {
	var gotBody string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "real-id",
		ClientSecret: "real-secret",
		ExtraParams: map[string]string{
			"grant_type":    "password",           // should be ignored
			"client_id":     "injected-id",        // should be ignored
			"client_secret": "injected-secret",    // should be ignored
			"scope":         "injected-scope",     // should be ignored
			"audience":      "https://api.vendor", // should be kept
		},
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
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
	if !strings.Contains(gotBody, "audience=") {
		t.Error("non-standard ExtraParams should be included")
	}
}

func TestGetCredentials_AuthModeBasic_SendsBasicHeader(t *testing.T) {
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

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "basic-id",
		ClientSecret: "basic-secret",
		AuthMode:     AuthModeBasic,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
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

func TestGetCredentials_ExpiryMargin_SubtractedFromTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	}))
	defer srv.Close()

	margin := 5 * time.Minute
	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		ExpiryMargin: margin,
	})

	before := time.Now()
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	cred, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
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

func TestGetCredentials_ExpiresIn_LessThanMargin_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","expires_in":30}`)
	}))
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		ExpiryMargin: 60 * time.Second, // 60s margin > 30s expires_in
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, contrib.ErrTokenExpiredOnArrival) {
		t.Errorf("error = %v, want errors.Is(ErrTokenExpiredOnArrival)", err)
	}
}

func TestGetCredentials_CustomHTTPClient(t *testing.T) {
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

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		HTTPClient:   customClient,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotCustomHeader != "custom-value" {
		t.Errorf("custom HTTP client not used: X-Custom-Test = %q, want %q",
			gotCustomHeader, "custom-value")
	}
}

// headerInjectTransport adds a custom header to verify the client is used.
type headerInjectTransport struct {
	key        string
	value      string
	underlying http.RoundTripper
}

func (t *headerInjectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(t.key, t.value)
	return t.underlying.RoundTrip(req)
}

func TestGetCredentials_NonSuccessResponse_LogsSanitizedOAuthError(t *testing.T) {
	responseBody := `{"error":"access_denied","error_description":"insufficient scope"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, responseBody)
	}))
	defer srv.Close()

	capture := &logCapture{}
	logger := slog.New(capture)

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Logger:       logger,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	_, err := cc.GetCredentials(ctx, sdk.TransactionContext{}, req)
	if err == nil {
		t.Fatal("expected error")
	}

	found := false
	for _, entry := range capture.getEntries() {
		if entry.message != "token endpoint error response" {
			continue
		}
		if entry.attrs["oauth_error"] != "access_denied" {
			t.Errorf("oauth_error = %q, want %q", entry.attrs["oauth_error"], "access_denied")
		}
		if entry.attrs["oauth_error_description"] != "insufficient scope" {
			t.Errorf("oauth_error_description = %q, want %q",
				entry.attrs["oauth_error_description"], "insufficient scope")
		}
		// Raw body must NOT be logged.
		if entry.attrs["body_prefix"] != "" {
			t.Error("body_prefix should not be present (raw body must not be logged)")
		}
		found = true
	}

	if !found {
		t.Error("expected 'token endpoint error response' log entry with oauth_error")
	}
}

func TestClientCredentials_Compliance(t *testing.T) {
	srv := httptest.NewServer(validTokenHandler())
	defer srv.Close()

	cc := NewClientCredentials(ClientCredentialsConfig{
		TokenURL:     srv.URL,
		ClientID:     "compliance-id",
		ClientSecret: "compliance-secret",
	})

	plugin := contrib.AsPlugin(cc)
	compliance.VerifyContract(t, plugin)
}

// logCapture is a test slog.Handler that captures log entries for inspection.
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

func (lc *logCapture) WithAttrs(_ []slog.Attr) slog.Handler { return lc }
func (lc *logCapture) WithGroup(_ string) slog.Handler      { return lc }

func (lc *logCapture) getEntries() []logEntry {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	cp := make([]logEntry, len(lc.entries))
	copy(cp, lc.entries)
	return cp
}

func TestNewClientCredentials_EmptyTokenURL_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty TokenURL")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "TokenURL must not be empty") {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()

	NewClientCredentials(ClientCredentialsConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		// TokenURL intentionally empty.
	})
}
