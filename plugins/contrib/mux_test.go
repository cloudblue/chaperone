// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/sdk"
	"github.com/cloudblue/chaperone/sdk/compliance"
)

// --- test helpers ---

// namedProvider returns a fixed credential with the given name as the
// Authorization header value. This lets tests assert which provider
// was selected by the mux.
type namedProvider struct {
	name string
}

func (p *namedProvider) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	return &sdk.Credential{
		Headers:   map[string]string{"Authorization": "Bearer " + p.name},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}, nil
}

type errProvider struct {
	err error
}

func (p *errProvider) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	return nil, p.err
}

type stubSigner struct {
	cert []byte
	err  error
}

func (s *stubSigner) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
	return s.cert, s.err
}

type stubModifier struct {
	action *sdk.ResponseAction
	err    error
}

func (s *stubModifier) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
	return s.action, s.err
}

func makeTestReq(ctx context.Context) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
	return req
}

// logCapture is an slog.Handler that records log entries for assertions.
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

// --- GetCredentials tests ---

func TestMux_GetCredentials_ExactVendorIDMatch(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{VendorID: "acme"}, &namedProvider{name: "acme"})
	mux.Handle(Route{VendorID: "globex"}, &namedProvider{name: "globex"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer acme" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer acme")
	}
}

func TestMux_GetCredentials_GlobVendorIDMatch(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{VendorID: "microsoft-*"}, &namedProvider{name: "microsoft"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "microsoft-azure"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer microsoft" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer microsoft")
	}
}

func TestMux_GetCredentials_TwoFieldBeatsOneField(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{VendorID: "microsoft-*"}, &namedProvider{name: "vendor-only"})
	mux.Handle(
		Route{EnvironmentID: "prod", VendorID: "microsoft-*"},
		&namedProvider{name: "env-and-vendor"},
	)

	ctx := context.Background()
	tx := sdk.TransactionContext{
		EnvironmentID: "prod",
		VendorID:      "microsoft-azure",
	}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer env-and-vendor" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer env-and-vendor")
	}
}

func TestMux_GetCredentials_TargetURLGlobMatch(t *testing.T) {
	mux := NewMux()
	mux.Handle(
		Route{TargetURL: "*.graph.microsoft.com/**"},
		&namedProvider{name: "graph"},
	)

	ctx := context.Background()
	tx := sdk.TransactionContext{
		TargetURL: "https://api.graph.microsoft.com/v1/users",
	}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer graph" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer graph")
	}
}

// --- fieldsMayOverlap unit tests ---

func TestFieldsMayOverlap(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		// Empty fields are wildcards at the route level, not a shared dimension.
		{"both empty", "", "", false},
		{"a empty", "", "acme", false},
		{"b empty", "acme", "", false},

		// Identical literals can match the same input.
		{"identical literals", "acme", "acme", true},

		// Different literals can never match the same input.
		{"disjoint literals", "acme", "globex", false},

		// Glob patterns: conservatively assume overlap.
		{"both globs", "ms-*", "ms-*", true},
		{"different globs", "acme-*", "globex-*", true},
		{"glob vs literal", "ms-*", "ms-azure", true},
		{"literal vs glob", "acme", "acme-*", true},
		{"double star vs literal", "**.example.com", "api.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fieldsMayOverlap(tt.a, tt.b); got != tt.want {
				t.Errorf("fieldsMayOverlap(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- routesMayOverlap unit tests ---

func TestRoutesMayOverlap(t *testing.T) {
	tests := []struct {
		name string
		a, b Route
		want bool
	}{
		// --- Single dimension (specificity 1) ---
		{
			name: "same dim, identical literal",
			a:    Route{VendorID: "acme"},
			b:    Route{VendorID: "acme"},
			want: true,
		},
		{
			name: "same dim, disjoint literals",
			a:    Route{VendorID: "acme"},
			b:    Route{VendorID: "globex"},
			want: false,
		},
		{
			name: "same dim, both globs",
			a:    Route{VendorID: "ms-*"},
			b:    Route{VendorID: "ms-azure"},
			want: true,
		},
		{
			name: "different dims, no shared field to disprove",
			a:    Route{VendorID: "acme"},
			b:    Route{TargetURL: "*.example.com/**"},
			want: true,
		},

		// --- Two dimensions (specificity 2) ---
		{
			name: "2-field, one shared dim disjoint literal, other shared dim has globs",
			a:    Route{VendorID: "acme", TargetURL: "*.api.com/**"},
			b:    Route{VendorID: "globex", TargetURL: "*.other.com/**"},
			want: false, // VendorID disjoint → impossible overlap
		},
		{
			name: "2-field, all shared dims identical literals",
			a:    Route{VendorID: "acme", EnvironmentID: "prod"},
			b:    Route{VendorID: "acme", EnvironmentID: "prod"},
			want: true,
		},
		{
			name: "2-field, all shared dims disjoint literals",
			a:    Route{VendorID: "acme", EnvironmentID: "prod"},
			b:    Route{VendorID: "globex", EnvironmentID: "staging"},
			want: false,
		},
		{
			name: "2-field, shared dim identical, other dim disjoint",
			a:    Route{VendorID: "acme", EnvironmentID: "prod"},
			b:    Route{VendorID: "acme", EnvironmentID: "staging"},
			want: false, // EnvironmentID disjoint
		},
		{
			name: "2-field, orthogonal dims, shared VendorID identical",
			a:    Route{VendorID: "acme", TargetURL: "*.api.com/**"},
			b:    Route{VendorID: "acme", EnvironmentID: "prod"},
			want: true, // shared VendorID matches, other dims unshared
		},
		{
			name: "2-field, both globs in all shared dims",
			a:    Route{VendorID: "ms-*", EnvironmentID: "prod-*"},
			b:    Route{VendorID: "ms-*", EnvironmentID: "staging-*"},
			want: true, // can't prove disjoint with globs
		},

		// --- MarketplaceID and ProductID dimensions ---
		{
			name: "same marketplace, disjoint literals",
			a:    Route{MarketplaceID: "MP-12345"},
			b:    Route{MarketplaceID: "MP-67890"},
			want: false,
		},
		{
			name: "same product, disjoint literals",
			a:    Route{ProductID: "MICROSOFT_SAAS"},
			b:    Route{ProductID: "AZURE"},
			want: false,
		},
		{
			name: "marketplace glob, may overlap",
			a:    Route{MarketplaceID: "MP-*"},
			b:    Route{MarketplaceID: "MP-12345"},
			want: true,
		},
		{
			name: "marketplace disjoint, product identical",
			a:    Route{MarketplaceID: "MP-12345", ProductID: "MICROSOFT_SAAS"},
			b:    Route{MarketplaceID: "MP-67890", ProductID: "MICROSOFT_SAAS"},
			want: false,
		},

		// --- Three dimensions (specificity 3) ---
		{
			name: "3-field, one dim disjoint literal",
			a:    Route{VendorID: "acme", TargetURL: "*.api.com/**", EnvironmentID: "prod"},
			b:    Route{VendorID: "globex", TargetURL: "*.api.com/**", EnvironmentID: "prod"},
			want: false, // VendorID disjoint
		},
		{
			name: "3-field, all dims may overlap",
			a:    Route{VendorID: "ms-*", TargetURL: "*.com/**", EnvironmentID: "prod"},
			b:    Route{VendorID: "ms-*", TargetURL: "*.net/**", EnvironmentID: "prod"},
			want: true, // all globs, can't prove disjoint
		},
		{
			name: "3-field, all identical literals",
			a:    Route{VendorID: "acme", TargetURL: "api.acme.com/v1", EnvironmentID: "prod"},
			b:    Route{VendorID: "acme", TargetURL: "api.acme.com/v1", EnvironmentID: "prod"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routesMayOverlap(tt.a, tt.b); got != tt.want {
				t.Errorf("routesMayOverlap(%+v, %+v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- Handle overlap warning integration test ---

func TestMux_Handle_OverlapWarning_FiredAtRegistrationNotDispatch(t *testing.T) {
	capture := &logCapture{}
	logger := slog.New(capture)
	mux := NewMux(WithLogger(logger))

	mux.Handle(Route{VendorID: "microsoft-*"}, &namedProvider{name: "first"})
	mux.Handle(Route{VendorID: "microsoft-azure"}, &namedProvider{name: "second"})

	// Warning must have been logged at registration time.
	found := false
	for _, entry := range capture.getEntries() {
		if entry.level == slog.LevelWarn && entry.message == "routes registered with equal specificity may overlap, first registered wins on tie" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning log at Handle() time for equal-specificity overlap, got none")
	}

	// First registered wins at dispatch time.
	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "microsoft-azure"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer first" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer first")
	}

	// No additional warnings at dispatch time.
	countBefore := len(capture.getEntries())
	_, _ = mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	countAfter := len(capture.getEntries())

	for _, entry := range capture.getEntries()[countBefore:countAfter] {
		if entry.level == slog.LevelWarn {
			t.Error("expected no warnings at dispatch time")
		}
	}
}

func TestMux_GetCredentials_NoMatch_DefaultFallback(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{VendorID: "acme"}, &namedProvider{name: "acme"})
	mux.Default(&namedProvider{name: "default"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "unknown-vendor"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer default" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer default")
	}
}

func TestMux_GetCredentials_NoMatch_NoDefault_ReturnsErrNoRouteMatch(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{VendorID: "acme"}, &namedProvider{name: "acme"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "unknown-vendor"}
	_, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if !errors.Is(err, ErrNoRouteMatch) {
		t.Errorf("error = %v, want ErrNoRouteMatch", err)
	}
}

func TestMux_GetCredentials_EmptyMux_NoDefault_ReturnsErrNoRouteMatch(t *testing.T) {
	mux := NewMux()

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "anything"}
	_, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if !errors.Is(err, ErrNoRouteMatch) {
		t.Errorf("error = %v, want ErrNoRouteMatch", err)
	}
}

func TestMux_GetCredentials_ProviderErrorPropagated(t *testing.T) {
	providerErr := errors.New("token fetch failed")
	mux := NewMux()
	mux.Handle(Route{VendorID: "acme"}, &errProvider{err: providerErr})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme"}
	_, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if !errors.Is(err, providerErr) {
		t.Errorf("error = %v, want %v", err, providerErr)
	}
}

func TestMux_GetCredentials_MarketplaceIDMatch(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{MarketplaceID: "MP-12345"}, &namedProvider{name: "eu"})
	mux.Handle(Route{MarketplaceID: "MP-67890"}, &namedProvider{name: "us"})

	ctx := context.Background()
	tx := sdk.TransactionContext{MarketplaceID: "MP-67890"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer us" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer us")
	}
}

func TestMux_GetCredentials_ProductIDMatch(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{ProductID: "MICROSOFT_SAAS"}, &namedProvider{name: "saas"})
	mux.Handle(Route{ProductID: "AZURE"}, &namedProvider{name: "azure"})

	ctx := context.Background()
	tx := sdk.TransactionContext{ProductID: "AZURE"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer azure" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer azure")
	}
}

func TestMux_GetCredentials_MarketplaceAndProductBeatsMarketplaceAlone(t *testing.T) {
	mux := NewMux()
	mux.Handle(Route{MarketplaceID: "MP-12345"}, &namedProvider{name: "marketplace-only"})
	mux.Handle(
		Route{MarketplaceID: "MP-12345", ProductID: "MICROSOFT_SAAS"},
		&namedProvider{name: "marketplace-and-product"},
	)

	ctx := context.Background()
	tx := sdk.TransactionContext{MarketplaceID: "MP-12345", ProductID: "MICROSOFT_SAAS"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer marketplace-and-product" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer marketplace-and-product")
	}
}

func TestMux_GetCredentials_HigherSpecificityWins_RegardlessOfOrder(t *testing.T) {
	// Register the more specific route first, less specific second.
	// The more specific should still win.
	mux := NewMux()
	mux.Handle(
		Route{EnvironmentID: "prod", VendorID: "acme"},
		&namedProvider{name: "specific"},
	)
	mux.Handle(Route{VendorID: "acme"}, &namedProvider{name: "general"})

	ctx := context.Background()
	tx := sdk.TransactionContext{EnvironmentID: "prod", VendorID: "acme"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer specific" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer specific")
	}
}

// --- SignCSR tests ---

func TestMux_SignCSR_WithoutSigner_ReturnsErrSigningNotConfigured(t *testing.T) {
	mux := NewMux()

	cert, err := mux.SignCSR(context.Background(), []byte("fake-csr"))
	if !errors.Is(err, ErrSigningNotConfigured) {
		t.Errorf("SignCSR() error = %v, want ErrSigningNotConfigured", err)
	}
	if cert != nil {
		t.Errorf("SignCSR() cert = %v, want nil", cert)
	}
}

func TestMux_SignCSR_WithSigner_Delegates(t *testing.T) {
	want := []byte("signed-cert-pem")
	mux := NewMux()
	mux.SetSigner(&stubSigner{cert: want})

	got, err := mux.SignCSR(context.Background(), []byte("fake-csr"))
	if err != nil {
		t.Fatalf("SignCSR() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("SignCSR() = %q, want %q", got, want)
	}
}

func TestMux_SignCSR_WithSigner_PropagatesError(t *testing.T) {
	signerErr := errors.New("CA unavailable")
	mux := NewMux()
	mux.SetSigner(&stubSigner{err: signerErr})

	_, err := mux.SignCSR(context.Background(), []byte("fake-csr"))
	if !errors.Is(err, signerErr) {
		t.Errorf("SignCSR() error = %v, want %v", err, signerErr)
	}
}

// --- ModifyResponse tests ---

func TestMux_ModifyResponse_WithoutModifier_ReturnsNilNil(t *testing.T) {
	mux := NewMux()

	action, err := mux.ModifyResponse(context.Background(), sdk.TransactionContext{}, nil)
	if err != nil {
		t.Errorf("ModifyResponse() error = %v, want nil", err)
	}
	if action != nil {
		t.Errorf("ModifyResponse() action = %v, want nil", action)
	}
}

func TestMux_ModifyResponse_WithModifier_Delegates(t *testing.T) {
	want := &sdk.ResponseAction{SkipErrorNormalization: true}
	mux := NewMux()
	mux.SetResponseModifier(&stubModifier{action: want})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
	}
	got, err := mux.ModifyResponse(context.Background(), sdk.TransactionContext{}, resp)
	if err != nil {
		t.Fatalf("ModifyResponse() error = %v", err)
	}
	if got == nil || got.SkipErrorNormalization != want.SkipErrorNormalization {
		t.Errorf("ModifyResponse() = %v, want %v", got, want)
	}
}

func TestMux_ModifyResponse_WithModifier_PropagatesError(t *testing.T) {
	modErr := errors.New("modifier failed")
	mux := NewMux()
	mux.SetResponseModifier(&stubModifier{err: modErr})

	_, err := mux.ModifyResponse(context.Background(), sdk.TransactionContext{}, nil)
	if !errors.Is(err, modErr) {
		t.Errorf("ModifyResponse() error = %v, want %v", err, modErr)
	}
}

// --- Compliance test ---

func TestMux_Compliance(t *testing.T) {
	mux := NewMux()
	mux.Default(&namedProvider{name: "compliance"})
	compliance.VerifyContract(t, mux)
}

// --- RouteRequest tests (RequestRouter implementation) ---

func TestMux_RouteRequest_ReturnsForward_ForForwardAction(t *testing.T) {
	m := NewMux()
	m.HandleForward(Route{VendorID: "microsoft-*"}, "company-b")

	action, err := m.RouteRequest(context.Background(),
		sdk.TransactionContext{VendorID: "microsoft-azure"},
		httptest.NewRequest("GET", "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("RouteRequest: %v", err)
	}
	if action == nil || action.ForwardTo != "company-b" {
		t.Errorf("action = %#v, want ForwardTo=company-b", action)
	}
}

func TestMux_RouteRequest_ReturnsNil_ForCredentialAction(t *testing.T) {
	m := NewMux()
	m.Handle(Route{VendorID: "microsoft-*"}, &namedProvider{name: "test"})

	action, err := m.RouteRequest(context.Background(),
		sdk.TransactionContext{VendorID: "microsoft-azure"},
		httptest.NewRequest("GET", "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("RouteRequest: %v", err)
	}
	if action != nil {
		t.Errorf("action = %#v, want nil for CredentialAction match", action)
	}
}

func TestMux_RouteRequest_ReturnsNil_NoMatch(t *testing.T) {
	m := NewMux()
	m.Default(&namedProvider{name: "fallback"})

	action, err := m.RouteRequest(context.Background(),
		sdk.TransactionContext{VendorID: "globex"},
		httptest.NewRequest("GET", "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("RouteRequest: %v", err)
	}
	if action != nil {
		t.Errorf("action = %#v, want nil when no forward route matches", action)
	}
}

// --- RouteRequest mandatory test matrix ---

func TestMux_RouteRequest_Matrix(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*Mux)
		tx          sdk.TransactionContext
		wantAction  *sdk.RouteAction
		wantErr     bool
		description string
	}{
		{
			name: "ForwardAction_MatchedWithData_ReturnsCorrectTarget",
			setup: func(m *Mux) {
				m.HandleForward(Route{VendorID: "acme", Data: map[string]string{"region": "us-east"}}, "acme-us-east")
			},
			tx:          sdk.TransactionContext{VendorID: "acme", Data: map[string]any{"region": "us-east"}},
			wantAction:  &sdk.RouteAction{ForwardTo: "acme-us-east"},
			description: "ForwardAction matched at specific Data dimension returns correct ForwardTo",
		},
		{
			name: "HigherSpecificityForwardBeatsLowerSpecificityCredential",
			setup: func(m *Mux) {
				m.Handle(Route{VendorID: "acme"}, &namedProvider{name: "general"})
				m.HandleForward(Route{VendorID: "acme", EnvironmentID: "prod"}, "acme-prod")
			},
			tx:          sdk.TransactionContext{VendorID: "acme", EnvironmentID: "prod"},
			wantAction:  &sdk.RouteAction{ForwardTo: "acme-prod"},
			description: "More specific ForwardAction wins over less specific CredentialAction",
		},
		{
			name: "HigherSpecificityCredentialBeatsLowerSpecificityForward",
			setup: func(m *Mux) {
				m.HandleForward(Route{VendorID: "acme"}, "general-acme")
				m.Handle(Route{VendorID: "acme", EnvironmentID: "prod"}, &namedProvider{name: "specific"})
			},
			tx:          sdk.TransactionContext{VendorID: "acme", EnvironmentID: "prod"},
			wantAction:  nil,
			description: "More specific CredentialAction wins over less specific ForwardAction (returns nil)",
		},
		{
			name: "TwoForwardActionsAtSameSpecificity_FirstRegisteredWins",
			setup: func(m *Mux) {
				m.HandleForward(Route{VendorID: "microsoft-*"}, "target-first")
				m.HandleForward(Route{VendorID: "microsoft-*"}, "target-second")
			},
			tx:          sdk.TransactionContext{VendorID: "microsoft-azure"},
			wantAction:  &sdk.RouteAction{ForwardTo: "target-first"},
			description: "Two ForwardActions matching at same specificity returns first registered",
		},
		{
			name: "ForwardActionWithEmptyTarget_ReturnsEmptyForwardTo",
			setup: func(m *Mux) {
				m.HandleForward(Route{VendorID: "test"}, "")
			},
			tx:          sdk.TransactionContext{VendorID: "test"},
			wantAction:  &sdk.RouteAction{ForwardTo: ""},
			description: "Empty target name on ForwardAction returns RouteAction with empty ForwardTo",
		},
		{
			name: "NilHTTPRequest_DoesNotPanic",
			setup: func(m *Mux) {
				m.HandleForward(Route{VendorID: "acme"}, "target-acme")
			},
			tx:          sdk.TransactionContext{VendorID: "acme"},
			wantAction:  &sdk.RouteAction{ForwardTo: "target-acme"},
			description: "nil http.Request argument does not panic",
		},
		{
			name: "NilTXData_WithDataDimensionRoute_NoMatch",
			setup: func(m *Mux) {
				m.HandleForward(Route{VendorID: "acme", Data: map[string]string{"region": "us"}}, "target-us")
			},
			tx:          sdk.TransactionContext{VendorID: "acme", Data: nil},
			wantAction:  nil,
			description: "nil tx.Data with a Data-dimension route does not match",
		},
		{
			name: "PreCancelledContext_StillReturnsAction",
			setup: func(m *Mux) {
				m.HandleForward(Route{VendorID: "acme"}, "target-acme")
			},
			tx:          sdk.TransactionContext{VendorID: "acme"},
			wantAction:  &sdk.RouteAction{ForwardTo: "target-acme"},
			description: "Pre-cancelled context still returns the same action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMux()
			tt.setup(m)

			ctx := context.Background()
			// For the "pre-cancelled context" case, cancel it before calling.
			if tt.name == "PreCancelledContext_StillReturnsAction" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}

			action, err := m.RouteRequest(ctx, tt.tx, nil) // nil req is acceptable
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantAction == nil {
				if action != nil {
					t.Errorf("action = %#v, want nil (%s)", action, tt.description)
				}
			} else {
				if action == nil {
					t.Errorf("action = nil, want %#v (%s)", tt.wantAction, tt.description)
				} else if action.ForwardTo != tt.wantAction.ForwardTo {
					t.Errorf("action.ForwardTo = %q, want %q (%s)", action.ForwardTo, tt.wantAction.ForwardTo, tt.description)
				}
			}
		})
	}
}

// --- RouteRequest RequestRouter compliance test ---

func TestMux_RouteRequest_Compliance(t *testing.T) {
	m := NewMux()
	m.HandleForward(Route{VendorID: "test"}, "target-test")
	compliance.VerifyRouter(t, m)
}

func TestNewMux_NilLogger_LazyResolution(t *testing.T) {
	m := NewMux()

	// logger field must be nil — no eager slog.Default() at construction.
	if m.logger != nil {
		t.Error("logger field should be nil when not provided; lazy resolution via log()")
	}
	if m.log() != slog.Default() {
		t.Error("log() should return slog.Default() when logger is nil")
	}
}

func TestNewMux_WithLogger_UsesExplicitLogger(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewMux(WithLogger(custom))

	if m.log() != custom {
		t.Error("log() should return the explicitly provided logger")
	}
}

// --- Action registration tests ---

// countOverlapWarnings returns how many overlap warnings the capture saw.
func countOverlapWarnings(entries []logEntry) int {
	const want = "routes registered with equal specificity may overlap, first registered wins on tie"
	n := 0
	for _, e := range entries {
		if e.level == slog.LevelWarn && e.message == want {
			n++
		}
	}
	return n
}

func TestMux_HandleForward_RegistersForwardAction(t *testing.T) {
	m := NewMux()
	m.HandleForward(Route{VendorID: "x"}, "company-b")

	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.entries))
	}
	fa, ok := m.entries[0].action.(ForwardAction)
	if !ok {
		t.Fatalf("action = %T, want ForwardAction", m.entries[0].action)
	}
	if fa.Target != "company-b" {
		t.Errorf("Target = %q, want %q", fa.Target, "company-b")
	}
	if m.entries[0].index != 0 {
		t.Errorf("index = %d, want 0", m.entries[0].index)
	}
}

func TestMux_HandleForward_MultipleRegistrations_PreserveOrder(t *testing.T) {
	m := NewMux()
	m.HandleForward(Route{VendorID: "a"}, "target-a")
	m.HandleForward(Route{VendorID: "b"}, "target-b")
	m.HandleForward(Route{VendorID: "c"}, "target-c")

	if len(m.entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(m.entries))
	}
	wantTargets := []string{"target-a", "target-b", "target-c"}
	for i, want := range wantTargets {
		fa, ok := m.entries[i].action.(ForwardAction)
		if !ok {
			t.Fatalf("entry[%d].action = %T, want ForwardAction", i, m.entries[i].action)
		}
		if fa.Target != want {
			t.Errorf("entry[%d].Target = %q, want %q", i, fa.Target, want)
		}
		if m.entries[i].index != i {
			t.Errorf("entry[%d].index = %d, want %d", i, m.entries[i].index, i)
		}
	}
}

func TestMux_Handle_RegistersCredentialAction(t *testing.T) {
	m := NewMux()
	provider := &namedProvider{name: "acme"}
	m.Handle(Route{VendorID: "acme"}, provider)

	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.entries))
	}
	ca, ok := m.entries[0].action.(CredentialAction)
	if !ok {
		t.Fatalf("action = %T, want CredentialAction", m.entries[0].action)
	}
	if ca.Provider != provider {
		t.Errorf("Provider = %v, want %v", ca.Provider, provider)
	}
}

func TestMux_MixedHandleAndHandleForward_AllRegisteredWithCorrectTypes(t *testing.T) {
	m := NewMux()
	prov := &namedProvider{name: "acme"}
	m.Handle(Route{VendorID: "acme"}, prov)
	m.HandleForward(Route{VendorID: "globex"}, "target-globex")
	m.Handle(Route{VendorID: "initech"}, &namedProvider{name: "initech"})
	m.HandleForward(Route{VendorID: "umbrella"}, "target-umbrella")

	if len(m.entries) != 4 {
		t.Fatalf("entries = %d, want 4", len(m.entries))
	}

	cases := []struct {
		idx      int
		wantType string
	}{
		{0, "credential"},
		{1, "forward"},
		{2, "credential"},
		{3, "forward"},
	}
	for _, tc := range cases {
		switch tc.wantType {
		case "credential":
			if _, ok := m.entries[tc.idx].action.(CredentialAction); !ok {
				t.Errorf("entry[%d].action = %T, want CredentialAction", tc.idx, m.entries[tc.idx].action)
			}
		case "forward":
			if _, ok := m.entries[tc.idx].action.(ForwardAction); !ok {
				t.Errorf("entry[%d].action = %T, want ForwardAction", tc.idx, m.entries[tc.idx].action)
			}
		}
	}
}

func TestMux_HandleForward_EmptyTarget_RegistersAsIs(t *testing.T) {
	// Decision: empty target is accepted by the mux — validation that the
	// target name is non-empty / references an existing forward_target lives
	// at config-load / cross-validation time. The mux is a passive registry.
	m := NewMux()
	m.HandleForward(Route{VendorID: "x"}, "")

	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.entries))
	}
	fa, ok := m.entries[0].action.(ForwardAction)
	if !ok {
		t.Fatalf("action = %T, want ForwardAction", m.entries[0].action)
	}
	if fa.Target != "" {
		t.Errorf("Target = %q, want empty string", fa.Target)
	}
}

// --- Overlap warning tests across action types ---

func TestMux_OverlapWarning_AcrossActionTypes(t *testing.T) {
	tests := []struct {
		name     string
		register func(m *Mux)
		wantWarn int
	}{
		{
			name: "two HandleForward with overlapping routes at same specificity",
			register: func(m *Mux) {
				m.HandleForward(Route{VendorID: "microsoft-*"}, "target-1")
				m.HandleForward(Route{VendorID: "microsoft-azure"}, "target-2")
			},
			wantWarn: 1,
		},
		{
			name: "Handle then HandleForward with overlapping routes at same specificity",
			register: func(m *Mux) {
				m.Handle(Route{VendorID: "microsoft-*"}, &namedProvider{name: "first"})
				m.HandleForward(Route{VendorID: "microsoft-azure"}, "target-2")
			},
			wantWarn: 1,
		},
		{
			name: "HandleForward then Handle with overlapping routes at same specificity",
			register: func(m *Mux) {
				m.HandleForward(Route{VendorID: "microsoft-*"}, "target-1")
				m.Handle(Route{VendorID: "microsoft-azure"}, &namedProvider{name: "second"})
			},
			wantWarn: 1,
		},
		{
			name: "two non-overlapping HandleForward (disjoint literals)",
			register: func(m *Mux) {
				m.HandleForward(Route{VendorID: "acme"}, "target-acme")
				m.HandleForward(Route{VendorID: "globex"}, "target-globex")
			},
			wantWarn: 0,
		},
		{
			name: "non-overlapping mixed actions (disjoint literals)",
			register: func(m *Mux) {
				m.Handle(Route{VendorID: "acme"}, &namedProvider{name: "acme"})
				m.HandleForward(Route{VendorID: "globex"}, "target-globex")
			},
			wantWarn: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &logCapture{}
			m := NewMux(WithLogger(slog.New(capture)))
			tt.register(m)

			if got := countOverlapWarnings(capture.getEntries()); got != tt.wantWarn {
				t.Errorf("overlap warnings = %d, want %d", got, tt.wantWarn)
			}
		})
	}
}

// --- GetCredentials fall-through tests for the new action model ---

func TestMux_GetCredentials_CredentialAction_DelegatesToProvider(t *testing.T) {
	m := NewMux()
	m.Handle(Route{VendorID: "acme"}, &namedProvider{name: "acme"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme"}
	cred, err := m.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer acme" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer acme")
	}
}

func TestMux_GetCredentials_ForwardAction_ReturnsErrUnexpectedForwardAction(t *testing.T) {
	m := NewMux()
	m.HandleForward(Route{VendorID: "acme"}, "company-b")

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme"}
	cred, err := m.GetCredentials(ctx, tx, makeTestReq(ctx))
	if !errors.Is(err, ErrUnexpectedForwardAction) {
		t.Errorf("error = %v, want ErrUnexpectedForwardAction", err)
	}
	if cred != nil {
		t.Errorf("cred = %v, want nil", cred)
	}
}

func TestMux_GetCredentials_NilProviderInCredentialAction_ReturnsErrNilCredentialProvider(t *testing.T) {
	m := NewMux()
	// Direct registration via Handle with nil provider.
	m.Handle(Route{VendorID: "acme"}, nil)

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme"}
	cred, err := m.GetCredentials(ctx, tx, makeTestReq(ctx))
	if !errors.Is(err, ErrNilCredentialProvider) {
		t.Errorf("error = %v, want ErrNilCredentialProvider", err)
	}
	if cred != nil {
		t.Errorf("cred = %v, want nil", cred)
	}
}

func TestMux_GetCredentials_ForwardActionMatched_DoesNotConsultFallback(t *testing.T) {
	// A ForwardAction match must NOT silently fall through to the default
	// provider — that would be a security regression.
	m := NewMux()
	m.HandleForward(Route{VendorID: "acme"}, "company-b")
	m.Default(&namedProvider{name: "fallback"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme"}
	_, err := m.GetCredentials(ctx, tx, makeTestReq(ctx))
	if !errors.Is(err, ErrUnexpectedForwardAction) {
		t.Errorf("error = %v, want ErrUnexpectedForwardAction (must not fall through to default)", err)
	}
}

func TestMux_GetCredentials_NoMatch_FallbackStillWorks(t *testing.T) {
	// Sanity: confirm fallback behavior still works after the refactor.
	m := NewMux()
	m.HandleForward(Route{VendorID: "acme"}, "company-b")
	m.Default(&namedProvider{name: "fallback"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "unknown-vendor"}
	cred, err := m.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer fallback" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer fallback")
	}
}

// --- Specificity interaction with mixed action types ---

func TestMux_GetCredentials_MoreSpecificForwardBeatsLessSpecificCredential(t *testing.T) {
	// Selection is by specificity, independent of action type. A more
	// specific ForwardAction wins over a less specific CredentialAction,
	// and the mux returns ErrUnexpectedForwardAction.
	m := NewMux()
	m.Handle(Route{VendorID: "acme"}, &namedProvider{name: "general"})
	m.HandleForward(
		Route{VendorID: "acme", EnvironmentID: "prod"},
		"target-acme-prod",
	)

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme", EnvironmentID: "prod"}
	_, err := m.GetCredentials(ctx, tx, makeTestReq(ctx))
	if !errors.Is(err, ErrUnexpectedForwardAction) {
		t.Errorf("error = %v, want ErrUnexpectedForwardAction", err)
	}
}

func TestMux_GetCredentials_MoreSpecificCredentialBeatsLessSpecificForward(t *testing.T) {
	m := NewMux()
	m.HandleForward(Route{VendorID: "acme"}, "target-general")
	m.Handle(
		Route{VendorID: "acme", EnvironmentID: "prod"},
		&namedProvider{name: "specific"},
	)

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "acme", EnvironmentID: "prod"}
	cred, err := m.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cred.Headers["Authorization"]; got != "Bearer specific" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer specific")
	}
}
