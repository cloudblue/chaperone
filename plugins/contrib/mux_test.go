// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
