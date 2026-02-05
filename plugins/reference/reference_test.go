// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package reference_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/plugins/reference"
	"github.com/cloudblue/chaperone/sdk"
	"github.com/cloudblue/chaperone/sdk/compliance"
)

// newTestRequest creates a test HTTP request, failing the test if creation fails.
func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create test request: %v", err)
	}
	return req
}

func TestReferencePlugin_Compliance(t *testing.T) {
	plugin := reference.New("testdata/credentials.json")
	compliance.VerifyContract(t, plugin)
}

// Compile-time interface verification
var _ sdk.Plugin = (*reference.Plugin)(nil)

func TestGetCredentials_BearerToken_ReturnsAuthorizationHeader(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	authHeader, ok := cred.Headers["Authorization"]
	if !ok {
		t.Fatal("expected Authorization header")
	}
	expected := "Bearer test-token-12345"
	if authHeader != expected {
		t.Errorf("got %q, want %q", authHeader, expected)
	}
}

func TestGetCredentials_APIKey_ReturnsCustomHeader(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "api-key-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	apiKey, ok := cred.Headers["X-API-Key"]
	if !ok {
		t.Fatal("expected X-API-Key header")
	}
	expected := "ak_live_xxxxx"
	if apiKey != expected {
		t.Errorf("got %q, want %q", apiKey, expected)
	}
}

func TestGetCredentials_BasicAuth_ReturnsAuthorizationHeader(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "basic-auth-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	authHeader, ok := cred.Headers["Authorization"]
	if !ok {
		t.Fatal("expected Authorization header")
	}
	expected := "Basic dXNlcm5hbWU6cGFzc3dvcmQ="
	if authHeader != expected {
		t.Errorf("got %q, want %q", authHeader, expected)
	}
}

func TestGetCredentials_UnknownVendor_ReturnsError(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "nonexistent-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err == nil {
		t.Fatal("expected error for unknown vendor")
	}
	if cred != nil {
		t.Errorf("expected nil credential, got %+v", cred)
	}
}

func TestGetCredentials_EmptyVendorID_ReturnsError(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: ""}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err == nil {
		t.Fatal("expected error for empty vendor ID")
	}
	if cred != nil {
		t.Errorf("expected nil credential, got %+v", cred)
	}
}

func TestGetCredentials_MissingFile_ReturnsError(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/nonexistent.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if cred != nil {
		t.Errorf("expected nil credential, got %+v", cred)
	}
}

func TestGetCredentials_CredentialHasValidExpiry(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
	if cred.IsExpired() {
		t.Error("credential should not be expired immediately after creation")
	}
	// Check TTL is approximately 60 minutes (with some tolerance)
	ttl := cred.TTL()
	if ttl < 59*time.Minute || ttl > 61*time.Minute { // 59-61 minutes in nanoseconds
		t.Errorf("TTL should be ~60 minutes, got %v", ttl)
	}
}

func TestSignCSR_ReturnsError(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	csrPEM := []byte("-----BEGIN CERTIFICATE REQUEST-----\ntest\n-----END CERTIFICATE REQUEST-----")

	// Act
	cert, err := plugin.SignCSR(ctx, csrPEM)

	// Assert
	if err == nil {
		t.Fatal("expected error from SignCSR in reference plugin")
	}
	if cert != nil {
		t.Errorf("expected nil certificate, got %v", cert)
	}
}

func TestModifyResponse_NoOp_ReturnsNilAction(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
	}
	resp.Header.Set("X-Custom-Header", "value")

	// Act
	action, err := plugin.ModifyResponse(ctx, tx, resp)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Errorf("expected nil action for default behavior, got %+v", action)
	}
	// Verify response is unchanged
	if resp.Header.Get("X-Custom-Header") != "value" {
		t.Error("response should not be modified")
	}
}

func TestGetCredentials_UnsupportedAuthType_ReturnsError(t *testing.T) {
	// This test verifies behavior when the JSON has an unsupported auth_type.
	// We use a separate test file for this.
	plugin := reference.New("testdata/unsupported_auth.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "bad-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err == nil {
		t.Fatal("expected error for unsupported auth type")
	}
	if cred != nil {
		t.Errorf("expected nil credential, got %+v", cred)
	}
}

func TestGetCredentials_APIKeyWithDefaultHeader(t *testing.T) {
	// Test that API key without explicit header_name uses X-API-Key default
	plugin := reference.New("testdata/api_key_default.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "default-header-vendor"}
	req := newTestRequest(t)

	// Act
	cred, err := plugin.GetCredentials(ctx, tx, req)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	apiKey, ok := cred.Headers["X-API-Key"]
	if !ok {
		t.Fatal("expected X-API-Key header (default)")
	}
	if apiKey != "default-key-value" {
		t.Errorf("got %q, want %q", apiKey, "default-key-value")
	}
}

func TestReloadCredentials_ClearsCache(t *testing.T) {
	// Arrange
	plugin := reference.New("testdata/credentials.json")
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req := newTestRequest(t)

	// Load credentials first
	_, err := plugin.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("unexpected error on first load: %v", err)
	}

	// Act
	plugin.ReloadCredentials()

	// Assert - should still work (reload from file)
	// Create a new request for the second call
	req2 := newTestRequest(t)
	cred, err := plugin.GetCredentials(ctx, tx, req2)
	if err != nil {
		t.Fatalf("unexpected error after reload: %v", err)
	}
	if cred == nil {
		t.Fatal("expected credential after reload")
	}
}
