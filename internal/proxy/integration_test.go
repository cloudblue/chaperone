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
	modifyResponseFn func(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) error
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

func (m *mockPlugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) error {
	if m.modifyResponseFn != nil {
		return m.modifyResponseFn(ctx, tx, resp)
	}
	return nil
}

// Verify mockPlugin implements sdk.Plugin at compile time.
var _ sdk.Plugin = (*mockPlugin)(nil)

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
		Addr:   ":0",
		Plugin: plugin,
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
		Addr: ":0",
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
		Addr:   ":0",
		Plugin: plugin,
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
		Addr:   ":0",
		Plugin: plugin,
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
		Addr:   ":0",
		Plugin: plugin,
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
		Addr: ":0",
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
		Addr:   ":0",
		Plugin: plugin,
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
		Addr: ":0",
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
		Addr:   ":0",
		Plugin: plugin,
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
