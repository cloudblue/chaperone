// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

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
//
// If the new route has the same specificity as an existing route and their
// patterns could overlap, a warning is logged at registration time. This
// is a static check — the mux cannot prove overlap in all cases, but
// equal specificity between two routes where at least one shared dimension
// contains a glob pattern is a strong signal. Disjoint literal values
// (e.g., VendorID "acme" vs "globex") are recognized as non-overlapping
// and do not trigger a warning.
func (m *Mux) Handle(route Route, provider sdk.CredentialProvider) {
	newSpec := route.Specificity()
	for _, e := range m.entries {
		if e.route.Specificity() == newSpec && routesMayOverlap(e.route, route) {
			m.logger.Warn("routes registered with equal specificity may overlap, first registered wins on tie",
				"existing_route", routeString(e.route),
				"new_route", routeString(route),
			)
			break
		}
	}

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
	best := m.match(tx)

	if best != nil {
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
	return nil, ErrSigningNotConfigured
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
// When multiple routes match at the same specificity, the first registered wins.
func (m *Mux) match(tx sdk.TransactionContext) *routeEntry {
	var best *routeEntry
	var bestSpec int

	for i := range m.entries {
		e := &m.entries[i]
		if !e.route.Matches(tx) {
			continue
		}

		spec := e.route.Specificity()

		if best == nil || spec > bestSpec {
			best = e
			bestSpec = spec
		}
	}
	return best
}

// routesMayOverlap reports whether two routes could potentially match the
// same request. For overlap, ALL shared dimensions (where both routes have
// a non-empty field) must potentially match the same input. If any shared
// dimension is provably disjoint (both are literal strings that differ),
// the routes cannot overlap regardless of the other dimensions.
//
// This is a conservative heuristic — it cannot prove overlap for glob
// patterns, so any shared dimension where at least one side contains a
// wildcard is treated as potentially overlapping. The warning this gates
// never changes routing behavior; it only alerts the operator.
func routesMayOverlap(a, b Route) bool {
	if a.VendorID != "" && b.VendorID != "" && !fieldsMayOverlap(a.VendorID, b.VendorID) {
		return false
	}
	if a.TargetURL != "" && b.TargetURL != "" && !fieldsMayOverlap(a.TargetURL, b.TargetURL) {
		return false
	}
	if a.EnvironmentID != "" && b.EnvironmentID != "" && !fieldsMayOverlap(a.EnvironmentID, b.EnvironmentID) {
		return false
	}
	return true
}

// fieldsMayOverlap reports whether two route field values could match the
// same input. Empty fields are wildcards at the route level (handled by
// specificity), so two empty fields don't count. Two non-empty literal
// strings that differ are provably disjoint.
func fieldsMayOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	// If neither contains a glob metacharacter, exact comparison suffices.
	if !containsGlob(a) && !containsGlob(b) {
		return a == b
	}
	// At least one is a pattern — conservatively assume overlap.
	return true
}

// containsGlob reports whether s contains glob metacharacters (* or **).
func containsGlob(s string) bool {
	return strings.ContainsRune(s, '*')
}

// routeString returns a human-readable representation of a route for log messages.
func routeString(r Route) string {
	var parts []string
	if r.VendorID != "" {
		parts = append(parts, "VendorID="+r.VendorID)
	}
	if r.TargetURL != "" {
		parts = append(parts, "TargetURL="+r.TargetURL)
	}
	if r.EnvironmentID != "" {
		parts = append(parts, "EnvironmentID="+r.EnvironmentID)
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
