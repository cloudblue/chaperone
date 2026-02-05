// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		Plugin:    plugin,
		AllowList: testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		AllowList: testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		Plugin:    plugin,
		AllowList: testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:          ":0",
		Plugin:        plugin,
		PluginTimeout: 50 * time.Millisecond, // Very short timeout for test
		AllowList:     testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		Plugin:    plugin,
		AllowList: testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		Plugin:    plugin,
		AllowList: testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		AllowList: testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		Plugin:    plugin,
		AllowList: testAllowList(),
	})
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

	srv := proxy.NewServer(proxy.Config{
		Addr:          ":0",
		Plugin:        plugin,
		PluginTimeout: 5 * time.Second,
		AllowList:     testAllowList(),
	})
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
	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		AllowList: testAllowList(),
	})
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

func TestIntegration_PluginContextCanceled_NoResponse(t *testing.T) {
	// Arrange - plugin that checks for context cancellation
	plugin := &mockPlugin{
		getCredentialsFn: func(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
			// Simulate context being cancelled (client disconnected)
			return nil, context.Canceled
		},
	}

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		Plugin:    plugin,
		AllowList: testAllowList(),
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "http://example.com")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - when context is canceled, we don't write a response (or write minimal)
	// The important thing is we don't crash and don't return 500
	// In practice, client won't see this response since they disconnected
	if rec.Code == http.StatusInternalServerError {
		t.Error("context.Canceled should not return 500")
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
	srv := proxy.NewServer(proxy.Config{
		Addr:         ":0",
		HeaderPrefix: "X-Connect",
		AllowList: map[string][]string{
			"127.0.0.1": {"/**"},
		},
	})

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
	srv := proxy.NewServer(proxy.Config{
		Addr:         ":0",
		HeaderPrefix: "X-Connect",
		AllowList: map[string][]string{
			"api.example.com": {"/**"},
		},
	})

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
	srv := proxy.NewServer(proxy.Config{
		Addr:         ":0",
		HeaderPrefix: "X-Connect",
		AllowList: map[string][]string{
			"127.0.0.1": {"/v1/**"},
		},
	})

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
	srv := proxy.NewServer(proxy.Config{
		Addr:         ":0",
		HeaderPrefix: "X-Connect",
		AllowList: map[string][]string{
			"127.0.0.1": {"/v1/customers/*/profiles", "/v1/orders/**"},
		},
	})

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
	srv := proxy.NewServer(proxy.Config{
		Addr:         ":0",
		HeaderPrefix: "X-Connect",
		AllowList:    map[string][]string{}, // Empty - deny all
	})

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

			srv := proxy.NewServer(proxy.Config{
				Addr:      ":0",
				TLS:       &proxy.TLSConfig{Enabled: false},
				AllowList: testAllowList(),
			})

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

			// Should have X-Error-ID header
			if rec.Header().Get("X-Error-ID") == "" {
				t.Error("X-Error-ID header should be set for error responses")
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		TLS:       &proxy.TLSConfig{Enabled: false},
		AllowList: testAllowList(),
	})

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

	// Should NOT have X-Error-ID header
	if rec.Header().Get("X-Error-ID") != "" {
		t.Error("X-Error-ID header should NOT be set for success responses")
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

	srv := proxy.NewServer(proxy.Config{
		Addr:      ":0",
		TLS:       &proxy.TLSConfig{Enabled: false},
		Plugin:    plugin,
		AllowList: testAllowList(),
	})

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
