// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chaperoneCtx "github.com/cloudblue/chaperone/internal/context"
	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/sdk"
)

// mockPlugin is a test double for sdk.Plugin.
type mockPlugin struct {
	getCredentialsFn func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error)
	signCSRFn        func(ctx context.Context, csrPEM []byte) ([]byte, error)
	modifyResponseFn func(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error)
}

func (m *mockPlugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
	if m.getCredentialsFn != nil {
		return m.getCredentialsFn(ctx, tx, req)
	}
	return nil, nil
}

func (m *mockPlugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
	if m.signCSRFn != nil {
		return m.signCSRFn(ctx, csrPEM)
	}
	return nil, errors.New("not implemented")
}

func (m *mockPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
	if m.modifyResponseFn != nil {
		return m.modifyResponseFn(ctx, tx, resp)
	}
	return nil, nil
}

// Verify mockPlugin implements sdk.Plugin at compile time.
var _ sdk.Plugin = (*mockPlugin)(nil)

// testAllowList returns a permissive allow list for testing.
func testAllowList() map[string][]string {
	return map[string][]string{
		"**": {"/**"}, // Allow all hosts and paths
	}
}

func TestIntegration_ProxyInjectsCredentials(t *testing.T) {
	// Arrange - mock backend that verifies injected headers
	var receivedAuthHeader string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"result": "success"}`)
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			return &sdk.Credential{
				Headers: map[string]string{
					"Authorization": "Bearer test-token-123",
				},
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedAuthHeader != "Bearer test-token-123" {
		t.Errorf("Authorization header = %q, want %q", receivedAuthHeader, "Bearer test-token-123")
	}
}

func TestIntegration_ProxyForwardsBody(t *testing.T) {
	// Arrange - mock backend that echoes the body
	var receivedBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body) // Echo back
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	requestBody := `{"action": "create", "data": {"name": "test"}}`
	req := httptest.NewRequest(http.MethodPost, "/proxy", strings.NewReader(requestBody))
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedBody != requestBody {
		t.Errorf("received body = %q, want %q", receivedBody, requestBody)
	}
}

func TestIntegration_PluginError_Returns500(t *testing.T) {
	// Arrange
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("backend should not be called when plugin fails")
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			return nil, errors.New("vault connection failed")
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestIntegration_PluginTimeout_Returns504(t *testing.T) {
	// Arrange
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("backend should not be called when plugin times out")
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			// Simulate slow plugin that exceeds timeout
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, errors.New("should not reach here")
			}
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.PluginTimeout = 50 * time.Millisecond // Very short timeout for test
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}
}

func TestIntegration_PluginReturnsNil_ForwardsWithoutInjection(t *testing.T) {
	// Arrange - plugin returns nil (Slow Path - direct mutation)
	var receivedAuthHeader string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			// Slow Path: mutate request directly
			req.Header.Set("Authorization", "Direct-Mutation-Token")
			return nil, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedAuthHeader != "Direct-Mutation-Token" {
		t.Errorf("Authorization = %q, want %q", receivedAuthHeader, "Direct-Mutation-Token")
	}
}

func TestIntegration_TransactionContextPassedToPlugin(t *testing.T) {
	// Arrange
	var receivedTx sdk.TransactionContext
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			receivedTx = tx
			return nil, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "vendor-123")
	req.Header.Set("X-Connect-Marketplace-ID", "marketplace-456")
	req.Header.Set("X-Connect-Product-ID", "product-789")
	req.Header.Set("X-Connect-Subscription-ID", "sub-abc")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if receivedTx.TargetURL != backend.URL {
		t.Errorf("TargetURL = %q, want %q", receivedTx.TargetURL, backend.URL)
	}
	if receivedTx.VendorID != "vendor-123" {
		t.Errorf("VendorID = %q, want %q", receivedTx.VendorID, "vendor-123")
	}
	if receivedTx.MarketplaceID != "marketplace-456" {
		t.Errorf("MarketplaceID = %q, want %q", receivedTx.MarketplaceID, "marketplace-456")
	}
	if receivedTx.ProductID != "product-789" {
		t.Errorf("ProductID = %q, want %q", receivedTx.ProductID, "product-789")
	}
	if receivedTx.SubscriptionID != "sub-abc" {
		t.Errorf("SubscriptionID = %q, want %q", receivedTx.SubscriptionID, "sub-abc")
	}
}

func TestIntegration_BackendError_Returns502(t *testing.T) {
	// Arrange - backend returns 500
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "Internal error with stack trace...")
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - we pass through the status code (error masking is Phase 2)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestIntegration_MultipleCredentialHeaders(t *testing.T) {
	// Arrange - plugin returns multiple headers
	var receivedHeaders http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			return &sdk.Credential{
				Headers: map[string]string{
					"Authorization":   "Bearer token-123",
					"X-API-Key":       "key-456",
					"X-Custom-Header": "custom-value",
				},
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - all headers injected
	if receivedHeaders.Get("Authorization") != "Bearer token-123" {
		t.Errorf("Authorization = %q, want %q", receivedHeaders.Get("Authorization"), "Bearer token-123")
	}
	if receivedHeaders.Get("X-API-Key") != "key-456" {
		t.Errorf("X-API-Key = %q, want %q", receivedHeaders.Get("X-API-Key"), "key-456")
	}
	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want %q", receivedHeaders.Get("X-Custom-Header"), "custom-value")
	}
}

func TestIntegration_PluginContextCancellation(t *testing.T) {
	// Arrange - test that plugin receives cancellable context
	contextReceived := make(chan context.Context, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			contextReceived <- ctx
			return nil, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.PluginTimeout = 5 * time.Second
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - context should have a deadline
	select {
	case ctx := <-contextReceived:
		if _, ok := ctx.Deadline(); !ok {
			t.Error("expected context to have a deadline")
		}
	case <-time.After(1 * time.Second):
		t.Error("plugin did not receive context")
	}
}

func TestIntegration_BackendUnreachable_Returns502(t *testing.T) {
	// Arrange - target URL that doesn't exist
	cfg := testConfig()
	cfg.AllowList = map[string][]string{"127.0.0.1:1": {"/**"}}
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	// Use a URL that will definitely fail to connect
	req.Header.Set("X-Connect-Target-URL", "http://127.0.0.1:1") // Port 1 is never open
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - should return 502 Bad Gateway
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestIntegration_PluginContextCanceled_Returns499(t *testing.T) {
	// Arrange - plugin that simulates client disconnect
	plugin := &mockPlugin{
		getCredentialsFn: func(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
			return nil, context.Canceled
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "http://example.com")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - 499 Client Closed Request (nginx convention for client disconnect)
	if rec.Code != proxy.StatusClientClosedRequest {
		t.Errorf("status = %d, want %d (StatusClientClosedRequest)", rec.Code, proxy.StatusClientClosedRequest)
	}
}

func TestIntegration_AllowList_ValidRequest_Passes(t *testing.T) {
	// Arrange - mock backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status": "allowed"}`)
	}))
	defer backend.Close()

	// Configure server with allow list
	cfg := testConfig()
	cfg.AllowList = map[string][]string{
		mustTargetHostPort(t, backend.URL): {"/**"},
	}
	srv := mustNewServer(t, cfg)

	// Create request to allowed host
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/v1/data")

	rec := httptest.NewRecorder()
	handler := srv.Handler()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - request should pass through
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "allowed") {
		t.Errorf("expected response body to contain 'allowed', got %q", rec.Body.String())
	}
}

func TestIntegration_AllowList_BlockedHost_Returns403(t *testing.T) {
	// Configure server with allow list (does NOT include evil.com)
	cfg := testConfig()
	cfg.AllowList = map[string][]string{
		"api.example.com": {"/**"},
	}
	srv := mustNewServer(t, cfg)

	// Create request to blocked host
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://evil.com/steal/data")

	rec := httptest.NewRecorder()
	handler := srv.Handler()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - should be forbidden
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "host not allowed") {
		t.Errorf("expected response to contain 'host not allowed', got %q", rec.Body.String())
	}
}

func TestIntegration_AllowList_BlockedPath_Returns403(t *testing.T) {
	// Arrange - mock backend (should not be reached)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("backend should not be reached for blocked path")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Configure server with restricted path
	cfg := testConfig()
	cfg.AllowList = map[string][]string{
		mustTargetHostPort(t, backend.URL): {"/v1/**"},
	}
	srv := mustNewServer(t, cfg)

	// Create request to blocked path
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/admin/users")

	rec := httptest.NewRecorder()
	handler := srv.Handler()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - should be forbidden
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "path not allowed") {
		t.Errorf("expected response to contain 'path not allowed', got %q", rec.Body.String())
	}
}

func TestIntegration_AllowList_GlobPatternMatches(t *testing.T) {
	// Arrange - mock backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"path": "`+r.URL.Path+`"}`)
	}))
	defer backend.Close()

	// Configure server with glob patterns
	cfg := testConfig()
	cfg.AllowList = map[string][]string{
		mustTargetHostPort(t, backend.URL): {"/v1/customers/*/profiles", "/v1/orders/**"},
	}
	srv := mustNewServer(t, cfg)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "single star matches one segment",
			path:       "/v1/customers/123/profiles",
			wantStatus: http.StatusOK,
		},
		{
			name:       "single star does not match nested",
			path:       "/v1/customers/123/456/profiles",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "double star matches nested paths",
			path:       "/v1/orders/2024/01/001",
			wantStatus: http.StatusOK,
		},
		{
			name:       "double star matches base path",
			path:       "/v1/orders",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", backend.URL+tt.path)

			rec := httptest.NewRecorder()
			handler := srv.Handler()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestIntegration_AllowList_EmptyList_DeniesAll(t *testing.T) {
	// Configure server without allow list (empty)
	cfg := testConfig()
	cfg.AllowList = map[string][]string{} // Empty - deny all
	srv := mustNewServer(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://any.host.com/any/path")

	rec := httptest.NewRecorder()
	handler := srv.Handler()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - should be forbidden
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d for empty allow list, got %d", http.StatusForbidden, rec.Code)
	}
}

// TestIntegration_UpstreamError_NormalizedResponse verifies that upstream 4xx/5xx
// errors are normalized per Design Spec Section 5.3 (Error Masking).
func TestIntegration_UpstreamError_NormalizedResponse(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	tests := []struct {
		name         string
		statusCode   int
		originalBody string
		wantError    string
	}{
		{
			name:         "500 with stack trace is normalized",
			statusCode:   500,
			originalBody: "panic: runtime error\ngoroutine 1:\nmain.go:42",
			wantError:    "Upstream service error",
		},
		{
			name:         "400 with internal details is normalized",
			statusCode:   400,
			originalBody: `{"error": "invalid field", "internal_id": "secret-123"}`,
			wantError:    "Request rejected by upstream service",
		},
		{
			name:         "503 service unavailable is normalized",
			statusCode:   503,
			originalBody: "Database connection failed: postgres://user:password@db.internal",
			wantError:    "Upstream service error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: create backend that returns error
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.originalBody))
			}))
			defer backend.Close()

			srv := mustNewServerForTarget(t, testConfig(), backend.URL)

			req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")

			rec := httptest.NewRecorder()
			handler := srv.Handler()

			// Act
			handler.ServeHTTP(rec, req)

			// Assert
			if rec.Code != tt.statusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.statusCode)
			}

			body := rec.Body.String()

			// Should contain normalized error message
			if !strings.Contains(body, tt.wantError) {
				t.Errorf("body should contain %q, got: %s", tt.wantError, body)
			}

			// Should NOT contain original sensitive content
			if strings.Contains(body, tt.originalBody) {
				t.Errorf("body should NOT contain original body %q, got: %s", tt.originalBody, body)
			}

			// Should be JSON
			if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
				t.Errorf("Content-Type should be application/json, got: %s", rec.Header().Get("Content-Type"))
			}
		})
	}
}

// TestIntegration_SuccessResponse_NotNormalized verifies that 2xx/3xx responses
// pass through unmodified.
func TestIntegration_SuccessResponse_NotNormalized(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	expectedBody := `{"data": "success", "id": 123}`

	// Arrange: create backend that returns success
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedBody))
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")

	rec := httptest.NewRecorder()
	handler := srv.Handler()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if body != expectedBody {
		t.Errorf("body = %q, want %q", body, expectedBody)
	}
}

// TestIntegration_PluginModifyResponse_RunsBeforeNormalization verifies the
// middleware chain order: Plugin.ModifyResponse runs BEFORE Core sanitization.
func TestIntegration_PluginModifyResponse_RunsBeforeNormalization(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	pluginCalled := false
	pluginSawStatusCode := 0

	plugin := &mockPlugin{
		modifyResponseFn: func(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
			pluginCalled = true
			pluginSawStatusCode = resp.StatusCode
			// Plugin can read/modify the response before normalization
			return nil, nil
		},
	}

	// Arrange: create backend that returns error
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("original error body"))
	}))
	defer backend.Close()

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")

	rec := httptest.NewRecorder()
	handler := srv.Handler()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if !pluginCalled {
		t.Error("Plugin.ModifyResponse should have been called")
	}
	if pluginSawStatusCode != http.StatusInternalServerError {
		t.Errorf("Plugin should see status %d, got %d", http.StatusInternalServerError, pluginSawStatusCode)
	}

	// Response should still be normalized (Core runs after plugin)
	body := rec.Body.String()
	if !strings.Contains(body, "Upstream service error") {
		t.Errorf("Response should be normalized, got: %s", body)
	}
}

// TestIntegration_ResponseSanitization_StripsSensitiveHeaders verifies that
// the Reflector strips sensitive headers from responses per Design Spec Section 5.3.
func TestIntegration_ResponseSanitization_StripsSensitiveHeaders(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	// Arrange - backend that returns sensitive headers (simulating credential leak)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a misconfigured backend that echoes auth headers
		w.Header().Set("Authorization", "Bearer leaked-secret")
		w.Header().Set("Set-Cookie", "session=leaked-session-id")
		w.Header().Set("X-Api-Key", "leaked-api-key")
		w.Header().Set("X-Auth-Token", "leaked-auth-token")
		// Safe headers that should be preserved
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-123")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"result": "success"}`)
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - sensitive headers MUST be stripped from response
	if rec.Header().Get("Authorization") != "" {
		t.Error("Authorization header should be stripped from response")
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Error("Set-Cookie header should be stripped from response")
	}
	if rec.Header().Get("X-Api-Key") != "" {
		t.Error("X-Api-Key header should be stripped from response")
	}
	if rec.Header().Get("X-Auth-Token") != "" {
		t.Error("X-Auth-Token header should be stripped from response")
	}

	// Assert - safe headers should be preserved
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type should be preserved, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Request-Id") != "req-123" {
		t.Errorf("X-Request-Id should be preserved, got %q", rec.Header().Get("X-Request-Id"))
	}
}

// TestIntegration_ResponseSanitization_CustomHeaders verifies that custom
// sensitive headers are merged with built-in defaults (not replacing them).
// Security: Default headers like Authorization MUST always be stripped,
// even when custom headers are configured. Per Design Spec Section 5.3.
func TestIntegration_ResponseSanitization_CustomHeaders(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	// Arrange - backend that returns both a custom header and a default sensitive header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Secret", "leaked-secret")
		w.Header().Set("Authorization", "Bearer token") // Default sensitive
		w.Header().Set("X-Safe-Header", "visible")      // Non-sensitive
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Configure server with custom sensitive headers (merged with defaults)
	cfg := testConfig()
	cfg.SensitiveHeaders = []string{"X-Custom-Secret"} // Merged with defaults
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - custom header stripped (user-provided)
	if rec.Header().Get("X-Custom-Secret") != "" {
		t.Error("X-Custom-Secret should be stripped when in custom sensitive list")
	}

	// Assert - Authorization stripped (from merged defaults)
	if rec.Header().Get("Authorization") != "" {
		t.Error("Authorization should be stripped (merged defaults always included)")
	}

	// Assert - non-sensitive header preserved
	if rec.Header().Get("X-Safe-Header") != "visible" {
		t.Errorf("X-Safe-Header should be preserved, got %q", rec.Header().Get("X-Safe-Header"))
	}
}

// TestIntegration_ResponseSanitization_StripsInjectedHeaders verifies that
// headers dynamically injected by the plugin are stripped from responses,
// even when they aren't in the static sensitive headers list.
//
// This covers the Design Spec Section 5.3 "Credential Reflection Protection":
// "The Proxy strips all Injection Headers" — meaning whatever the plugin
// actually injected, not just well-known names.
func TestIntegration_ResponseSanitization_StripsInjectedHeaders(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	// Arrange - backend that echoes the injected header back in its response
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate ISV echoing the vendor-specific auth header
		if v := r.Header.Get("X-Vendor-Magic-Token"); v != "" {
			w.Header().Set("X-Vendor-Magic-Token", v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-456")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"result": "ok"}`)
	}))
	defer backend.Close()

	// Plugin injects a non-standard header (not in DefaultSensitiveHeaders)
	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			return &sdk.Credential{
				Headers: map[string]string{
					"X-Vendor-Magic-Token": "super-secret-vendor-key",
				},
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - the echoed injection header MUST be stripped
	if rec.Header().Get("X-Vendor-Magic-Token") != "" {
		t.Error("X-Vendor-Magic-Token should be stripped from response (injected header reflection)")
	}

	// Assert - safe headers preserved
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type should be preserved, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Request-Id") != "req-456" {
		t.Errorf("X-Request-Id should be preserved, got %q", rec.Header().Get("X-Request-Id"))
	}
}

// TestIntegration_ResponseSanitization_InjectedAndStaticCombined verifies
// that both static (well-known) and dynamic (per-request injected) headers
// are stripped from the same response.
func TestIntegration_ResponseSanitization_InjectedAndStaticCombined(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ISV echoes both the standard and custom injected headers
		w.Header().Set("Authorization", "Bearer leaked")
		w.Header().Set("X-Vendor-Token", "vendor-secret-echoed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			return &sdk.Credential{
				Headers: map[string]string{
					"Authorization":  "Bearer my-token",
					"X-Vendor-Token": "vendor-secret",
				},
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - both static and dynamic injection headers stripped
	if rec.Header().Get("Authorization") != "" {
		t.Error("Authorization should be stripped (static sensitive list)")
	}
	if rec.Header().Get("X-Vendor-Token") != "" {
		t.Error("X-Vendor-Token should be stripped (dynamically injected header)")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type should be preserved, got %q", rec.Header().Get("Content-Type"))
	}
}

// TestIntegration_ResponseSanitization_SlowPath_NoInjectedHeaders verifies
// that when the plugin uses the Slow Path (returns nil credential), the
// dynamic stripping is a safe no-op and static stripping still works.
func TestIntegration_ResponseSanitization_SlowPath_NoInjectedHeaders(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Authorization", "Bearer leaked")
		w.Header().Set("X-Custom-Header", "should-stay")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Slow Path: plugin mutates request directly, returns nil credential
	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			req.Header.Set("X-Slow-Path-Auth", "direct-mutation")
			return nil, nil // Slow Path
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - static stripping still works
	if rec.Header().Get("Authorization") != "" {
		t.Error("Authorization should be stripped by static reflector")
	}
	// Non-sensitive header preserved (no dynamic keys were registered)
	if rec.Header().Get("X-Custom-Header") != "should-stay" {
		t.Errorf("X-Custom-Header should be preserved, got %q", rec.Header().Get("X-Custom-Header"))
	}
}

// TestIntegration_ResponseSanitization_SlowPath_AutoDetectsInjectedHeaders verifies
// that the Core automatically detects headers injected by Slow Path plugins
// (via pre/post snapshot diffing) and strips them from responses — without
// the plugin needing to call WithInjectedHeaders() or WithSecretValue().
//
// This is the defense-in-depth guarantee: even if a plugin author forgets
// the manual context calls, credential reflection protection still works.
func TestIntegration_ResponseSanitization_SlowPath_AutoDetectsInjectedHeaders(t *testing.T) {
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	// Backend echoes the injected header back in the response
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Vendor-Hmac-Sig"); v != "" {
			w.Header().Set("X-Vendor-Hmac-Sig", v) // ISV echoes it
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-789")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"result": "ok"}`)
	}))
	defer backend.Close()

	// Slow Path plugin: directly mutates headers, does NOT call
	// WithSecretValue or WithInjectedHeaders. The Core must detect this.
	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			// Simulate HMAC signing — adds a non-standard header
			req.Header.Set("X-Vendor-Hmac-Sig", "hmac-sha256-signature-value")
			return nil, nil // Slow Path
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - the echoed injection header MUST be stripped
	// even though the plugin never called WithInjectedHeaders
	if rec.Header().Get("X-Vendor-Hmac-Sig") != "" {
		t.Error("X-Vendor-Hmac-Sig should be auto-detected and stripped from response (Slow Path defense-in-depth)")
	}

	// Assert - safe headers preserved
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type should be preserved, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Request-Id") != "req-789" {
		t.Errorf("X-Request-Id should be preserved, got %q", rec.Header().Get("X-Request-Id"))
	}
}

// =============================================================================
// Middleware Stack Integration Tests
// =============================================================================
//
// These tests exercise the REAL middleware stack via Handler() to catch
// ordering regressions in withMiddleware(). They verify the end-to-end
// interaction between TraceIDMiddleware, RequestLoggerMiddleware, and
// PanicRecovery — not individual middlewares in isolation.

// TestHandlerStack_TraceID_PropagatedToBackend verifies the full trace ID
// lifecycle through the real middleware stack: TraceIDMiddleware generates a
// UUID → stores it in context → handleProxy reads it → reverse proxy forwards
// it to the vendor in the request header.
func TestHandlerStack_TraceID_PropagatedToBackend(t *testing.T) {
	// Arrange — backend captures the trace header it receives
	var receivedTraceID string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceID = r.Header.Get("Connect-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	req.Header.Set("Connect-Request-ID", "upstream-trace-abc")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert — backend MUST receive the exact trace ID from upstream
	if receivedTraceID != "upstream-trace-abc" {
		t.Errorf("backend received trace_id = %q, want %q", receivedTraceID, "upstream-trace-abc")
	}
}

// TestHandlerStack_TraceID_GeneratedAndPropagated verifies that when no
// trace header is provided, the middleware generates a UUIDv4 and propagates
// it to the backend (not an empty string or timestamp-based ID).
func TestHandlerStack_TraceID_GeneratedAndPropagated(t *testing.T) {
	var receivedTraceID string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceID = r.Header.Get("Connect-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	// No Connect-Request-ID header — should be generated
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert — backend must receive a generated UUIDv4 (36 chars: 8-4-4-4-12)
	if receivedTraceID == "" {
		t.Fatal("backend received empty trace_id, expected generated UUID")
	}
	if len(receivedTraceID) != 36 {
		t.Errorf("generated trace_id length = %d, want 36 (UUIDv4): %q", len(receivedTraceID), receivedTraceID)
	}
	if strings.HasPrefix(receivedTraceID, "trace-") {
		t.Errorf("trace_id should be UUIDv4, not timestamp-based: %q", receivedTraceID)
	}
}

// TestHandlerStack_TraceID_ConsistentAcrossLogAndBackend verifies that the
// trace ID logged by RequestLoggerMiddleware matches the one sent to the
// backend — proving the context propagation chain works end-to-end.
func TestHandlerStack_TraceID_ConsistentAcrossLogAndBackend(t *testing.T) {
	var receivedTraceID string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceID = r.Header.Get("Connect-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Capture log output
	var logBuf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	origLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(origLogger)

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	req.Header.Set("Connect-Request-ID", "consistency-check-123")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert — same trace ID in backend AND in log output
	if receivedTraceID != "consistency-check-123" {
		t.Errorf("backend trace_id = %q, want %q", receivedTraceID, "consistency-check-123")
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, `"trace_id":"consistency-check-123"`) {
		t.Errorf("log output should contain trace_id, got:\n%s", logOutput)
	}
}

// TestHandlerStack_PanicRecovery_StillWorks verifies that panic recovery
// works correctly through the full Handler() stack (not manually composed).
// This catches ordering regressions where TraceIDMiddleware might break
// the PanicRecovery → RequestLogger interaction.
func TestHandlerStack_PanicRecovery_StillWorks(t *testing.T) {
	// Use a plugin that panics to trigger recovery inside handleProxy
	panickingPlugin := &mockPlugin{
		getCredentialsFn: func(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
			panic("plugin exploded")
		},
	}

	cfg := testConfig()
	cfg.Plugin = panickingPlugin
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.example.com/v1")
	req.Header.Set("Connect-Request-ID", "panic-test-456")
	rec := httptest.NewRecorder()

	// Act — should NOT panic (recovered by middleware)
	handler.ServeHTTP(rec, req)

	// Assert — response should be 500, not a crash
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// --- Server-Timing Integration Tests ---

// TestIntegration_ServerTiming_PresentOnSuccess verifies Server-Timing header
// is added to successful responses per Design Spec Section 8.3.3.
func TestIntegration_ServerTiming_PresentOnSuccess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result": "success"}`))
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present")
	}

	// Verify format contains all three components
	if !strings.Contains(header, "plugin;dur=") {
		t.Errorf("Header = %q, want to contain 'plugin;dur='", header)
	}
	if !strings.Contains(header, "upstream;dur=") {
		t.Errorf("Header = %q, want to contain 'upstream;dur='", header)
	}
	if !strings.Contains(header, "overhead;dur=") {
		t.Errorf("Header = %q, want to contain 'overhead;dur='", header)
	}
}

// TestIntegration_ServerTiming_PresentOnError verifies Server-Timing header
// is added even when upstream returns an error.
func TestIntegration_ServerTiming_PresentOnError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present on error responses")
	}
}

// TestIntegration_ServerTiming_ReflectsPluginDuration verifies that plugin
// execution time is accurately reflected in the Server-Timing header.
func TestIntegration_ServerTiming_ReflectsPluginDuration(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Plugin that takes measurable time
	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			time.Sleep(50 * time.Millisecond)
			return &sdk.Credential{
				Headers:   map[string]string{"Authorization": "Bearer token"},
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")

	// Parse plugin duration from header
	parts := strings.Split(header, ", ")
	var pluginDur float64
	for _, part := range parts {
		if strings.HasPrefix(part, "plugin;dur=") {
			_, err := fmt.Sscanf(part, "plugin;dur=%f", &pluginDur)
			if err != nil {
				t.Fatalf("failed to parse plugin duration from %q: %v", part, err)
			}
			break
		}
	}

	// Plugin should have taken at least ~50ms (generous lower bound for CI)
	if pluginDur < 40.0 || pluginDur > 500.0 {
		t.Errorf("plugin duration = %.2fms, want between 40ms and 500ms", pluginDur)
	}
}

// TestIntegration_ServerTiming_ReflectsUpstreamDuration verifies that upstream
// latency is accurately reflected in the Server-Timing header.
func TestIntegration_ServerTiming_ReflectsUpstreamDuration(t *testing.T) {
	// Backend that takes measurable time
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")

	// Parse upstream duration from header
	parts := strings.Split(header, ", ")
	var upstreamDur float64
	for _, part := range parts {
		if strings.HasPrefix(part, "upstream;dur=") {
			_, err := fmt.Sscanf(part, "upstream;dur=%f", &upstreamDur)
			if err != nil {
				t.Fatalf("failed to parse upstream duration from %q: %v", part, err)
			}
			break
		}
	}

	// Upstream should have taken at least ~30ms (generous lower bound for CI)
	if upstreamDur < 20.0 || upstreamDur > 500.0 {
		t.Errorf("upstream duration = %.2fms, want between 20ms and 500ms", upstreamDur)
	}
}

// TestIntegration_ServerTiming_NoPluginShowsZeroDuration verifies that when
// no plugin is configured, the plugin duration is zero.
func TestIntegration_ServerTiming_NoPluginShowsZeroDuration(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// No plugin configured
	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")

	// Plugin duration should be near zero (no plugin execution)
	if !strings.Contains(header, "plugin;dur=0.00") {
		t.Errorf("Header = %q, want plugin;dur=0.00 when no plugin", header)
	}
}

// TestIntegration_ServerTiming_PresentOnPluginError verifies Server-Timing header
// is added when the plugin returns an error (500), with non-zero plugin duration.
func TestIntegration_ServerTiming_PresentOnPluginError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("backend should not be called when plugin fails")
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			time.Sleep(20 * time.Millisecond)
			return nil, errors.New("vault connection failed")
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present on plugin error responses")
	}

	// Plugin ran, so plugin;dur should be > 0. Upstream was never reached, so upstream;dur=0.00.
	if !strings.Contains(header, "upstream;dur=0.00") {
		t.Errorf("Header = %q, want upstream;dur=0.00 when upstream was never reached", header)
	}
}

// TestIntegration_ServerTiming_PresentOnPluginTimeout verifies Server-Timing header
// is added when the plugin times out (504), with plugin duration reflecting the timeout.
func TestIntegration_ServerTiming_PresentOnPluginTimeout(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("backend should not be called when plugin times out")
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, errors.New("should not reach here")
			}
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.PluginTimeout = 50 * time.Millisecond
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present on plugin timeout responses")
	}

	// Upstream was never reached
	if !strings.Contains(header, "upstream;dur=0.00") {
		t.Errorf("Header = %q, want upstream;dur=0.00 when upstream was never reached", header)
	}
}

// TestIntegration_ServerTiming_PresentOnAllowListRejection verifies Server-Timing
// is present when the request is rejected by the AllowList (403 Forbidden).
// In this path, plugin and upstream phases never execute.
func TestIntegration_ServerTiming_PresentOnAllowListRejection(t *testing.T) {
	// Server with an allow list that does NOT include evil.com
	cfg := testConfig()
	cfg.AllowList = map[string][]string{
		"api.example.com": {"/**"},
	}
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://evil.com/steal/data")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present on 403 Forbidden")
	}

	// Plugin and upstream never ran
	if !strings.Contains(header, "plugin;dur=0.00") {
		t.Errorf("Header = %q, want plugin;dur=0.00 when AllowList rejects", header)
	}
	if !strings.Contains(header, "upstream;dur=0.00") {
		t.Errorf("Header = %q, want upstream;dur=0.00 when AllowList rejects", header)
	}
}

// TestIntegration_ServerTiming_PresentOnBadRequest verifies Server-Timing
// is present when the request is malformed (400 Bad Request).
// Missing X-Connect-Target-URL header triggers this path.
func TestIntegration_ServerTiming_PresentOnBadRequest(t *testing.T) {
	cfg := testConfig()
	cfg.AllowList = map[string][]string{"127.0.0.1:1": {"/**"}}
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	// Missing X-Connect-Target-URL → 400
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present on 400 Bad Request")
	}

	// Plugin and upstream never ran
	if !strings.Contains(header, "plugin;dur=0.00") {
		t.Errorf("Header = %q, want plugin;dur=0.00 on bad request", header)
	}
	if !strings.Contains(header, "upstream;dur=0.00") {
		t.Errorf("Header = %q, want upstream;dur=0.00 on bad request", header)
	}
}

// TestIntegration_ServerTiming_BadGatewayIncludesHeader verifies that
// Server-Timing is present even when the upstream is unreachable.
func TestIntegration_ServerTiming_BadGatewayIncludesHeader(t *testing.T) {
	cfg := testConfig()
	cfg.AllowList = map[string][]string{"127.0.0.1:1": {"/**"}}
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	// Port 1 is never open - will cause 502 Bad Gateway
	req.Header.Set("X-Connect-Target-URL", "http://127.0.0.1:1/api")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present on 502 Bad Gateway")
	}
}

// TestIntegration_ServerTiming_SurvivesPanic verifies the key architectural
// invariant: TimingMiddleware wraps PanicRecoveryMiddleware so that panic-induced
// 500 responses still carry the Server-Timing header. This ensures performance
// attribution is available even when the handler panics.
func TestIntegration_ServerTiming_SurvivesPanic(t *testing.T) {
	// Plugin that sleeps (measurable duration) then panics
	panickingPlugin := &mockPlugin{
		getCredentialsFn: func(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
			time.Sleep(20 * time.Millisecond)
			panic("plugin exploded during credentials")
		},
	}

	cfg := testConfig()
	cfg.Plugin = panickingPlugin
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.example.com/v1")
	req.Header.Set("Connect-Request-ID", "panic-timing-test")
	rec := httptest.NewRecorder()

	// Act — should NOT crash (recovered by PanicRecoveryMiddleware)
	handler.ServeHTTP(rec, req)

	// Assert — response should be 500
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Assert — Server-Timing header MUST be present (key invariant)
	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header must be present on panic 500 responses")
	}

	// Assert — all three components present
	if !strings.Contains(header, "plugin;dur=") {
		t.Errorf("Header = %q, want to contain 'plugin;dur='", header)
	}
	if !strings.Contains(header, "upstream;dur=") {
		t.Errorf("Header = %q, want to contain 'upstream;dur='", header)
	}
	if !strings.Contains(header, "overhead;dur=") {
		t.Errorf("Header = %q, want to contain 'overhead;dur='", header)
	}

	// Note: plugin;dur=0.00 is expected here. RecordPlugin runs after
	// GetCredentials returns, but a panic unwinds the stack before that
	// line executes. The key invariant is header presence, not duration
	// accuracy on panic paths.
}

// TestIntegration_OpsEndpoints_NoServerTimingHeader verifies that /_ops
// endpoints do NOT emit Server-Timing headers (scoped to /proxy only).
func TestIntegration_OpsEndpoints_NoServerTimingHeader(t *testing.T) {
	srv := mustNewServer(t, testConfig())
	handler := srv.Handler()

	for _, path := range []string{"/_ops/health", "/_ops/version"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if header := rec.Header().Get("Server-Timing"); header != "" {
			t.Errorf("%s should NOT have Server-Timing header, got %q", path, header)
		}
	}
}

// TestIntegration_GlobalPanicRecovery_ProtectsAllRoutes is a structural
// regression test for the bug where PanicRecoveryMiddleware was accidentally
// inside a comment in withMiddleware(), leaving /_ops endpoints unprotected.
// It tests withMiddleware() directly with a panicking handler to verify
// the global PanicRecoveryMiddleware is in the chain.
func TestIntegration_GlobalPanicRecovery_ProtectsAllRoutes(t *testing.T) {
	srv := mustNewServer(t, testConfig())

	// Wrap a panicking handler through the real withMiddleware() chain
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("simulated /_ops panic")
	})
	handler := srv.WithMiddlewareForTesting(panicking)

	req := httptest.NewRequest(http.MethodGet, "/_ops/health", nil)
	rec := httptest.NewRecorder()

	// Act — must NOT crash the process
	handler.ServeHTTP(rec, req)

	// Assert — PanicRecoveryMiddleware should catch the panic and return 500
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (global PanicRecoveryMiddleware should catch panic)", rec.Code, http.StatusInternalServerError)
	}

	// Assert — response should be the JSON format from PanicRecoveryMiddleware
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

// TestIntegration_ContextHeaders_StrippedBeforeForwarding verifies that
// internal context headers (X-Connect-*) are NOT forwarded to the target.
// These headers carry Connect metadata (vendor ID, marketplace layout, etc.)
// that must not leak to external vendors. Regression test for the
// context-header-leaking bug.
func TestIntegration_ContextHeaders_StrippedBeforeForwarding(t *testing.T) {
	// Arrange - backend captures all received headers
	receivedHeaders := make(http.Header)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			receivedHeaders[k] = v
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := testConfig()
	cfg.Plugin = &mockPlugin{}
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/v1/resource")
	req.Header.Set("X-Connect-Vendor-ID", "vendor-123")
	req.Header.Set("X-Connect-Marketplace-ID", "MP-12345")
	req.Header.Set("X-Connect-Product-ID", "PRD-67890")
	req.Header.Set("X-Connect-Subscription-ID", "SUB-ABCDE")
	req.Header.Set("X-Connect-Context-Data", "e30=") // base64 "{}"
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - request should succeed
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Assert - context headers must NOT be forwarded
	for _, suffix := range chaperoneCtx.HeaderSuffixes() {
		h := "X-Connect" + suffix
		if val := receivedHeaders.Get(h); val != "" {
			t.Errorf("context header %q leaked to target with value %q", h, val)
		}
	}
}

// TestIntegration_TraceHeader_PreservedOnForwarding verifies that the trace
// header (Connect-Request-ID) is preserved when forwarding to the target.
// Per Design Spec §8.3, this header enables cross-company distributed tracing.
func TestIntegration_TraceHeader_PreservedOnForwarding(t *testing.T) {
	// Arrange - backend captures the trace header
	var receivedTraceID string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceID = r.Header.Get("Connect-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := testConfig()
	cfg.Plugin = &mockPlugin{}
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/v1/resource")
	req.Header.Set("Connect-Request-ID", "trace-abc-123")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedTraceID != "trace-abc-123" {
		t.Errorf("trace header = %q, want %q", receivedTraceID, "trace-abc-123")
	}
}

// TestIntegration_CustomPrefix_ContextHeadersStripped verifies that context
// header stripping works with a custom header prefix (ADR-005: configurable naming).
func TestIntegration_CustomPrefix_ContextHeadersStripped(t *testing.T) {
	// Arrange - backend captures all received headers
	receivedHeaders := make(http.Header)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			receivedHeaders[k] = v
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := testConfig()
	cfg.HeaderPrefix = "X-MyPlatform"
	cfg.Plugin = &mockPlugin{}
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-MyPlatform-Target-URL", backend.URL+"/v1/resource")
	req.Header.Set("X-MyPlatform-Vendor-ID", "vendor-custom")
	req.Header.Set("X-MyPlatform-Marketplace-ID", "MP-CUSTOM")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - request should succeed
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Assert - custom-prefixed context headers must NOT be forwarded
	for _, suffix := range chaperoneCtx.HeaderSuffixes() {
		h := "X-MyPlatform" + suffix
		if val := receivedHeaders.Get(h); val != "" {
			t.Errorf("context header %q leaked to target with value %q", h, val)
		}
	}
}

// TestIntegration_NonContextHeaders_PreservedOnForwarding verifies that
// non-context headers (Content-Type, Accept, etc.) are preserved when
// forwarding to the target. Only X-Connect-* headers should be stripped.
func TestIntegration_NonContextHeaders_PreservedOnForwarding(t *testing.T) {
	// Arrange - backend captures specific headers
	var receivedContentType, receivedAccept string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := testConfig()
	cfg.Plugin = &mockPlugin{}
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", strings.NewReader(`{"key":"value"}`))
	req.Header.Set("X-Connect-Target-URL", backend.URL+"/v1/resource")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedContentType, "application/json")
	}
	if receivedAccept != "application/json" {
		t.Errorf("Accept = %q, want %q", receivedAccept, "application/json")
	}
}

// =============================================================================
// Phase 2: DEBUG logging for credential injection
// =============================================================================

func TestIntegration_FastPath_LogsCredentialInjection(t *testing.T) {
	// Arrange - capture DEBUG log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	plugin := &mockPlugin{
		getCredentialsFn: func(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
			return &sdk.Credential{
				Headers:   map[string]string{"Authorization": "Bearer test-token"},
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "VA-test")
	req.Header.Set("X-Connect-Marketplace-ID", "MP-test")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Assert - "credentials injected" DEBUG log with Fast Path fields
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"msg":"credentials injected"`) {
		t.Errorf("expected credentials injected log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"credential_path":"fast"`) {
		t.Errorf("expected credential_path=fast in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"injected_header_count":1`) {
		t.Errorf("expected injected_header_count=1 in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"vendor_id":"VA-test"`) {
		t.Errorf("expected vendor_id in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"plugin_duration_ms"`) {
		t.Errorf("expected plugin_duration_ms in log, got: %s", logOutput)
	}
}

func TestIntegration_SlowPath_LogsCredentialInjection(t *testing.T) {
	// Arrange - capture DEBUG log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Slow path: plugin mutates request directly and returns nil credential
	plugin := &mockPlugin{
		getCredentialsFn: func(_ context.Context, _ sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			req.Header.Set("Authorization", "Bearer slow-token")
			return nil, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServerForTarget(t, cfg, backend.URL)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", backend.URL)
	req.Header.Set("X-Connect-Vendor-ID", "VA-slow")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Assert - "credentials injected" DEBUG log with Slow Path fields
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"msg":"credentials injected"`) {
		t.Errorf("expected credentials injected log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"credential_path":"slow"`) {
		t.Errorf("expected credential_path=slow in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"vendor_id":"VA-slow"`) {
		t.Errorf("expected vendor_id in log, got: %s", logOutput)
	}
}

// =============================================================================
// Phase 6: DEBUG logpoints for context parsing
// =============================================================================

func TestProxy_ContextParsed_DebugLog_LogsHostOnly(t *testing.T) {
	// Arrange - capture DEBUG log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	srv := mustNewServerForTarget(t, testConfig(), backend.URL)
	handler := srv.Handler()

	// URL with a sensitive path segment and query params — only the host should appear in logs
	targetURL := backend.URL + "/v1/users/alice@example.com?api_key=supersecret&token=abc123"

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", targetURL)
	req.Header.Set("X-Connect-Vendor-ID", "VA-test")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - only the host appears; path, query, and userinfo must not leak
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"msg":"transaction context parsed"`) {
		t.Errorf("expected 'transaction context parsed' debug log, got: %s", logOutput)
	}
	if strings.Contains(logOutput, "supersecret") {
		t.Errorf("log must not contain sensitive query value 'supersecret', got: %s", logOutput)
	}
	if strings.Contains(logOutput, "api_key") {
		t.Errorf("log must not contain query param name 'api_key', got: %s", logOutput)
	}
	if strings.Contains(logOutput, "token=") {
		t.Errorf("log must not contain 'token=' query param, got: %s", logOutput)
	}
	if strings.Contains(logOutput, "alice@example.com") {
		t.Errorf("log must not contain path segment 'alice@example.com', got: %s", logOutput)
	}
	if strings.Contains(logOutput, "/v1/users") {
		t.Errorf("log must not contain URL path, got: %s", logOutput)
	}
}

// =============================================================================
// Phase 3: Status 499 on client disconnect
// =============================================================================

func TestIntegration_ClientDisconnect_LogsStatus499(t *testing.T) {
	// Arrange - capture log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	plugin := &mockPlugin{
		getCredentialsFn: func(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
			return nil, context.Canceled
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "http://example.com")
	req.Header.Set("X-Connect-Vendor-ID", "VA-disconnect")
	req.Header.Set("Connect-Request-ID", "trace-499-test")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - RequestLoggerMiddleware logs status 499 (not the default 200)
	if rec.Code != proxy.StatusClientClosedRequest {
		t.Errorf("response status = %d, want %d", rec.Code, proxy.StatusClientClosedRequest)
	}
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"status":499`) {
		t.Errorf("log should contain status 499, got: %s", logOutput)
	}
}
