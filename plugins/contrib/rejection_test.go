// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

// rejectionRecorder is a provider that records the status code of each
// response it receives through CredentialRejected.
type rejectionRecorder struct {
	stubProvider
	calls []int
}

func (r *rejectionRecorder) CredentialRejected(_ context.Context, _ sdk.TransactionContext, resp *http.Response) {
	r.calls = append(r.calls, resp.StatusCode)
}

func TestMux_ModifyResponse_401_NotifiesMatchedProvider(t *testing.T) {
	matched := &rejectionRecorder{}
	other := &rejectionRecorder{}
	fallback := &rejectionRecorder{}

	mux := NewMux()
	mux.Handle(Route{VendorID: "microsoft-*"}, matched)
	mux.Handle(Route{VendorID: "acme"}, other)
	mux.Default(fallback)

	resp := &http.Response{StatusCode: http.StatusUnauthorized}
	tx := sdk.TransactionContext{VendorID: "microsoft-365"}

	action, err := mux.ModifyResponse(context.Background(), tx, resp)
	if err != nil || action != nil {
		t.Fatalf("ModifyResponse() = (%v, %v), want (nil, nil)", action, err)
	}

	if len(matched.calls) != 1 || matched.calls[0] != http.StatusUnauthorized {
		t.Errorf("matched provider got CredentialRejected calls %v, want one with status 401", matched.calls)
	}
	if len(other.calls) != 0 || len(fallback.calls) != 0 {
		t.Errorf("other providers were notified: other=%d fallback=%d, want 0",
			len(other.calls), len(fallback.calls))
	}
}

func TestMux_ModifyResponse_401_NoRouteMatch_NotifiesDefaultProvider(t *testing.T) {
	routed := &rejectionRecorder{}
	fallback := &rejectionRecorder{}

	mux := NewMux()
	mux.Handle(Route{VendorID: "acme"}, routed)
	mux.Default(fallback)

	_, err := mux.ModifyResponse(context.Background(),
		sdk.TransactionContext{VendorID: "globex"}, &http.Response{StatusCode: http.StatusUnauthorized})
	if err != nil {
		t.Fatalf("ModifyResponse() error = %v", err)
	}

	if len(fallback.calls) != 1 {
		t.Errorf("default provider got %d CredentialRejected calls, want 1", len(fallback.calls))
	}
	if len(routed.calls) != 0 {
		t.Errorf("non-matching provider got %d CredentialRejected calls, want 0", len(routed.calls))
	}
}

func TestMux_ModifyResponse_Non401_DoesNotNotify(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusInternalServerError} {
		provider := &rejectionRecorder{}
		mux := NewMux()
		mux.Default(provider)

		if _, err := mux.ModifyResponse(context.Background(), sdk.TransactionContext{}, &http.Response{StatusCode: status}); err != nil {
			t.Fatalf("status %d: ModifyResponse() error = %v", status, err)
		}
		if len(provider.calls) != 0 {
			t.Errorf("status %d: provider got %d CredentialRejected calls, want 0", status, len(provider.calls))
		}
	}
}

func TestMux_ModifyResponse_401_ProvidersThatCannotBeNotified(t *testing.T) {
	tx := sdk.TransactionContext{VendorID: "acme"}

	tests := []struct {
		name  string
		setup func(m *Mux)
	}{
		{"no routes and no default", func(*Mux) {}},
		{"provider without CredentialRejected", func(m *Mux) { m.Handle(Route{VendorID: "acme"}, &stubProvider{}) }},
		{"nil provider", func(m *Mux) { m.Handle(Route{VendorID: "acme"}, nil) }},
		{"forward action", func(m *Mux) { m.HandleForward(Route{VendorID: "acme"}, "company-b") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := NewMux()
			tt.setup(mux)

			action, err := mux.ModifyResponse(context.Background(), tx, &http.Response{StatusCode: http.StatusUnauthorized})
			if err != nil || action != nil {
				t.Errorf("ModifyResponse() = (%v, %v), want (nil, nil)", action, err)
			}
		})
	}
}

func TestMux_ModifyResponse_401_StillDelegatesToModifier(t *testing.T) {
	provider := &rejectionRecorder{}
	want := &sdk.ResponseAction{SkipErrorNormalization: true}

	mux := NewMux()
	mux.Default(provider)
	mux.SetResponseModifier(&stubModifier{action: want})

	got, err := mux.ModifyResponse(context.Background(), sdk.TransactionContext{}, &http.Response{StatusCode: http.StatusUnauthorized})
	if err != nil {
		t.Fatalf("ModifyResponse() error = %v", err)
	}
	if got != want {
		t.Errorf("ModifyResponse() = %v, want the modifier's action %v", got, want)
	}
	if len(provider.calls) != 1 {
		t.Errorf("provider got %d CredentialRejected calls, want 1", len(provider.calls))
	}
}

func TestAsPlugin_ModifyResponse_401_NotifiesProvider(t *testing.T) {
	provider := &rejectionRecorder{}
	plugin := AsPlugin(provider)
	ctx := context.Background()

	if _, err := plugin.ModifyResponse(ctx, sdk.TransactionContext{}, &http.Response{StatusCode: http.StatusOK}); err != nil {
		t.Fatalf("ModifyResponse(200) error = %v", err)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("provider got %d CredentialRejected calls for a 200, want 0", len(provider.calls))
	}

	resp := &http.Response{StatusCode: http.StatusUnauthorized}
	action, err := plugin.ModifyResponse(ctx, sdk.TransactionContext{}, resp)
	if err != nil || action != nil {
		t.Fatalf("ModifyResponse(401) = (%v, %v), want (nil, nil)", action, err)
	}
	if len(provider.calls) != 1 || provider.calls[0] != http.StatusUnauthorized {
		t.Errorf("provider got CredentialRejected calls %v, want one with status 401", provider.calls)
	}
}

func TestAsPlugin_ModifyResponse_401_ProviderWithoutHandler_ReturnsNil(t *testing.T) {
	plugin := AsPlugin(&stubProvider{})

	action, err := plugin.ModifyResponse(context.Background(), sdk.TransactionContext{}, &http.Response{StatusCode: http.StatusUnauthorized})
	if err != nil || action != nil {
		t.Errorf("ModifyResponse() = (%v, %v), want (nil, nil)", action, err)
	}
}
