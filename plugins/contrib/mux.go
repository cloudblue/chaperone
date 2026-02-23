// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/cloudblue/chaperone/sdk"
)

// Compile-time check that Mux implements sdk.Plugin.
var _ sdk.Plugin = (*Mux)(nil)

// routeEntry binds a route pattern to its credential provider,
// preserving registration order for tie-breaking.
type routeEntry struct {
	route    Route
	provider sdk.CredentialProvider
	index    int
}

// Mux is a request multiplexer that dispatches incoming requests to
// the most specific matching [sdk.CredentialProvider]. It implements
// [sdk.Plugin] and can be passed directly to chaperone.Run().
//
// Routes are matched by specificity: a route with more non-empty fields
// wins over one with fewer. When multiple routes match with equal
// specificity, the first registered route wins and a warning is logged.
//
// Example:
//
//	mux := contrib.NewMux()
//	mux.Handle(contrib.Route{VendorID: "microsoft-*"}, msProvider)
//	mux.Handle(contrib.Route{VendorID: "acme"}, acmeProvider)
//	mux.Default(fallbackProvider)
type Mux struct {
	entries  []routeEntry
	fallback sdk.CredentialProvider
	signer   sdk.CertificateSigner
	modifier sdk.ResponseModifier
	logger   *slog.Logger
}

// MuxOption configures a [Mux] at construction time.
type MuxOption func(*Mux)

// WithLogger sets the logger used by the mux for warnings (e.g., tie-breaking).
// If not set, [slog.Default] is used.
func WithLogger(l *slog.Logger) MuxOption {
	return func(m *Mux) { m.logger = l }
}

// NewMux creates a new request multiplexer. Use [Mux.Handle] and
// [Mux.Default] to register routes before serving traffic.
func NewMux(opts ...MuxOption) *Mux {
	m := &Mux{
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Handle registers a route that dispatches matching requests to the
// given provider. Routes are evaluated by specificity at dispatch time,
// not registration order — but registration order breaks ties.
func (m *Mux) Handle(route Route, provider sdk.CredentialProvider) {
	m.entries = append(m.entries, routeEntry{
		route:    route,
		provider: provider,
		index:    len(m.entries),
	})
}

// Default sets a fallback provider used when no registered route matches.
func (m *Mux) Default(provider sdk.CredentialProvider) {
	m.fallback = provider
}

// SetSigner configures the certificate signer delegate. Without a signer,
// [Mux.SignCSR] returns an error.
func (m *Mux) SetSigner(signer sdk.CertificateSigner) {
	m.signer = signer
}

// SetResponseModifier configures the response modifier delegate. Without
// a modifier, [Mux.ModifyResponse] returns (nil, nil) — the safe default.
func (m *Mux) SetResponseModifier(modifier sdk.ResponseModifier) {
	m.modifier = modifier
}

// GetCredentials dispatches the request to the most specific matching
// route's provider. If no route matches, it falls back to the default
// provider. Returns [ErrNoRouteMatch] if nothing matches and no default
// is configured.
func (m *Mux) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
	best, tied := m.match(tx)

	if best != nil {
		if tied {
			m.logger.Warn("multiple routes matched with equal specificity, using first registered",
				"vendor_id", tx.VendorID,
				"target_url", tx.TargetURL,
				"environment_id", tx.EnvironmentID,
			)
		}
		return best.provider.GetCredentials(ctx, tx, req)
	}

	if m.fallback != nil {
		return m.fallback.GetCredentials(ctx, tx, req)
	}

	return nil, ErrNoRouteMatch
}

// SignCSR delegates to the configured signer, or returns an error if
// no signer has been set via [Mux.SetSigner].
func (m *Mux) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
	if m.signer != nil {
		return m.signer.SignCSR(ctx, csrPEM)
	}
	return nil, fmt.Errorf("certificate signing not configured")
}

// ModifyResponse delegates to the configured response modifier, or
// returns (nil, nil) if none has been set via [Mux.SetResponseModifier].
func (m *Mux) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
	if m.modifier != nil {
		return m.modifier.ModifyResponse(ctx, tx, resp)
	}
	return nil, nil
}

// match finds the best matching route entry for the given transaction.
// It returns the best entry and whether a tie was detected (multiple
// matches at the same highest specificity).
func (m *Mux) match(tx sdk.TransactionContext) (best *routeEntry, tied bool) {
	for i := range m.entries {
		e := &m.entries[i]
		if !e.route.Matches(tx) {
			continue
		}

		spec := e.route.Specificity()

		if best == nil || spec > best.route.Specificity() {
			best = e
			tied = false
		} else if spec == best.route.Specificity() {
			tied = true
		}
	}
	return best, tied
}
