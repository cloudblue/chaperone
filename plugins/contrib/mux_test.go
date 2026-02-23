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

func TestMux_GetCredentials_EqualSpecificity_FirstRegisteredWins_WarningLogged(t *testing.T) {
	capture := &logCapture{}
	logger := slog.New(capture)
	mux := NewMux(WithLogger(logger))

	mux.Handle(Route{VendorID: "microsoft-*"}, &namedProvider{name: "first"})
	mux.Handle(Route{VendorID: "microsoft-azure"}, &namedProvider{name: "second"})

	ctx := context.Background()
	tx := sdk.TransactionContext{VendorID: "microsoft-azure"}
	cred, err := mux.GetCredentials(ctx, tx, makeTestReq(ctx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First registered wins.
	if got := cred.Headers["Authorization"]; got != "Bearer first" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer first")
	}

	// Warning must be logged.
	found := false
	for _, entry := range capture.getEntries() {
		if entry.level == slog.LevelWarn && entry.message == "multiple routes matched with equal specificity, using first registered" {
			found = true
			if entry.attrs["vendor_id"] != "microsoft-azure" {
				t.Errorf("log vendor_id = %q, want %q", entry.attrs["vendor_id"], "microsoft-azure")
			}
		}
	}
	if !found {
		t.Error("expected warning log for equal-specificity tie, got none")
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

func TestMux_SignCSR_WithoutSigner_ReturnsError(t *testing.T) {
	mux := NewMux()

	cert, err := mux.SignCSR(context.Background(), []byte("fake-csr"))
	if err == nil {
		t.Error("SignCSR() should return error when no signer configured")
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
