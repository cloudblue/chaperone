// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package compliance provides a test kit for validating Plugin implementations.
//
// Distributors should use VerifyContract in their test suite to ensure
// their plugin correctly implements the SDK interfaces and handles
// edge cases properly.
//
// Example usage:
//
//	func TestMyPluginCompliance(t *testing.T) {
//	    plugin := &MyPlugin{}
//	    compliance.VerifyContract(t, plugin)
//	}
package compliance

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/sdk"
)

// VerifyContract runs a comprehensive test suite against a Plugin implementation.
//
// It verifies that the plugin:
//   - Implements all required interfaces
//   - Handles nil/empty inputs gracefully (no panics)
//   - Returns valid Credential objects (when using Fast Path)
//   - Handles context cancellation appropriately
//
// Parameters:
//   - t: Testing context for reporting failures
//   - p: The Plugin implementation to test
func VerifyContract(t *testing.T, p sdk.Plugin) {
	t.Helper()

	t.Run("CredentialProvider", func(t *testing.T) {
		testCredentialProvider(t, p)
	})

	t.Run("CertificateSigner", func(t *testing.T) {
		testCertificateSigner(t, p)
	})

	t.Run("ResponseModifier", func(t *testing.T) {
		testResponseModifier(t, p)
	})
}

func testCredentialProvider(t *testing.T, p sdk.CredentialProvider) {
	t.Helper()

	t.Run("handles empty context", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("GetCredentials panicked with empty context: %v", r)
			}
		}()

		ctx := context.Background()
		tx := sdk.TransactionContext{}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

		// Should not panic - may return error, that's OK
		_, _ = p.GetCredentials(ctx, tx, req)
	})

	t.Run("handles cancelled context", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("GetCredentials panicked with cancelled context: %v", r)
			}
		}()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		tx := sdk.TransactionContext{VendorID: "test-vendor"}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

		// Should not panic - expected to return error or handle gracefully
		_, _ = p.GetCredentials(ctx, tx, req)
	})

	t.Run("credential expiry is valid", func(t *testing.T) {
		ctx := context.Background()
		tx := sdk.TransactionContext{
			VendorID:  "test-vendor",
			ProductID: "test-product",
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

		cred, err := p.GetCredentials(ctx, tx, req)

		// If returning a credential (Fast Path), validate it
		if err == nil && cred != nil {
			if cred.ExpiresAt.IsZero() {
				t.Error("Credential.ExpiresAt should not be zero")
			}
			if cred.ExpiresAt.Before(time.Now()) {
				t.Error("Credential.ExpiresAt should be in the future")
			}
		}
	})
}

func testCertificateSigner(t *testing.T, p sdk.CertificateSigner) {
	t.Helper()

	t.Run("handles empty CSR", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SignCSR panicked with empty CSR: %v", r)
			}
		}()

		ctx := context.Background()

		// Should return error, not panic
		_, err := p.SignCSR(ctx, []byte{})
		if err == nil {
			t.Log("SignCSR accepted empty CSR (may be intentional for mock implementations)")
		}
	})

	t.Run("handles nil CSR", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SignCSR panicked with nil CSR: %v", r)
			}
		}()

		ctx := context.Background()

		// Should return error, not panic
		_, err := p.SignCSR(ctx, nil)
		if err == nil {
			t.Log("SignCSR accepted nil CSR (may be intentional for mock implementations)")
		}
	})
}

func testResponseModifier(t *testing.T, p sdk.ResponseModifier) {
	t.Helper()

	t.Run("handles nil response", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ModifyResponse panicked with nil response: %v", r)
			}
		}()

		ctx := context.Background()
		tx := sdk.TransactionContext{}

		// Should return error or handle gracefully, not panic
		_ = p.ModifyResponse(ctx, tx, nil)
	})

	t.Run("handles valid response", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ModifyResponse panicked with valid response: %v", r)
			}
		}()

		ctx := context.Background()
		tx := sdk.TransactionContext{VendorID: "test-vendor"}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
		}

		err := p.ModifyResponse(ctx, tx, resp)
		if err != nil {
			t.Logf("ModifyResponse returned error: %v (may be expected)", err)
		}
	})
}
