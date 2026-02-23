// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/sdk"
	"github.com/cloudblue/chaperone/sdk/compliance"
)

// stubProvider is a minimal CredentialProvider for testing the adapter.
type stubProvider struct {
	cred *sdk.Credential
	err  error
}

func (s *stubProvider) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	return s.cred, s.err
}

func TestAsPlugin_DelegatesGetCredentials(t *testing.T) {
	want := &sdk.Credential{
		Headers:   map[string]string{"Authorization": "Bearer test-token"},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	provider := &stubProvider{cred: want}
	plugin := AsPlugin(provider)

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "test-vendor"}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)

	got, err := plugin.GetCredentials(ctx, tx, req)
	if err != nil {
		t.Fatalf("GetCredentials() error = %v", err)
	}
	if got.Headers["Authorization"] != want.Headers["Authorization"] {
		t.Errorf("GetCredentials() header = %q, want %q",
			got.Headers["Authorization"], want.Headers["Authorization"])
	}
}

func TestAsPlugin_SignCSR_ReturnsError(t *testing.T) {
	provider := &stubProvider{}
	plugin := AsPlugin(provider)

	_, err := plugin.SignCSR(context.Background(), []byte("fake-csr"))
	if err == nil {
		t.Error("SignCSR() should return error when no signer configured")
	}
}

func TestAsPlugin_ModifyResponse_ReturnsNil(t *testing.T) {
	provider := &stubProvider{}
	plugin := AsPlugin(provider)

	action, err := plugin.ModifyResponse(context.Background(), sdk.TransactionContext{}, nil)
	if err != nil {
		t.Errorf("ModifyResponse() error = %v, want nil", err)
	}
	if action != nil {
		t.Errorf("ModifyResponse() action = %v, want nil", action)
	}
}

func TestAsPlugin_Compliance(t *testing.T) {
	provider := &stubProvider{
		cred: &sdk.Credential{
			Headers:   map[string]string{"Authorization": "Bearer compliance-token"},
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}
	plugin := AsPlugin(provider)
	compliance.VerifyContract(t, plugin)
}
