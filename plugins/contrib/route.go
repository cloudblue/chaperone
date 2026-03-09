// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"strings"

	"github.com/cloudblue/chaperone/sdk"
)

// Route defines matching criteria for dispatching requests to a
// CredentialProvider. Each non-empty field must match the corresponding
// TransactionContext field. Empty fields act as wildcards.
//
// Fields support glob patterns with * (single-level) and ** (recursive).
// For example:
//
//	Route{VendorID: "microsoft-*"}                           // matches microsoft-azure, microsoft-365
//	Route{EnvironmentID: "prod", VendorID: "microsoft-*"}   // 2-field route, higher specificity
//	Route{TargetURL: "*.graph.microsoft.com/**"}             // matches any Graph API path
//	Route{MarketplaceID: "MP-*", ProductID: "MICROSOFT_SAAS"} // matches by marketplace and product
type Route struct {
	// VendorID matches against TransactionContext.VendorID.
	// Supports glob patterns (e.g., "microsoft-*").
	VendorID string

	// MarketplaceID matches against TransactionContext.MarketplaceID.
	// Supports glob patterns (e.g., "MP-*").
	MarketplaceID string

	// ProductID matches against TransactionContext.ProductID.
	// Supports glob patterns (e.g., "MICROSOFT_*").
	ProductID string

	// TargetURL matches against TransactionContext.TargetURL.
	// The URL scheme (e.g., "https://") is stripped before matching.
	// Supports glob patterns (e.g., "*.graph.microsoft.com/**").
	TargetURL string

	// EnvironmentID matches against TransactionContext.EnvironmentID.
	// Supports glob patterns (e.g., "prod-*").
	EnvironmentID string
}

// Specificity returns the number of non-empty fields in the route.
// A higher specificity means a more specific match. Used by the mux
// to prefer more specific routes when multiple routes match.
func (r Route) Specificity() int {
	n := 0
	if r.VendorID != "" {
		n++
	}
	if r.MarketplaceID != "" {
		n++
	}
	if r.ProductID != "" {
		n++
	}
	if r.TargetURL != "" {
		n++
	}
	if r.EnvironmentID != "" {
		n++
	}
	return n
}

// Matches reports whether the route matches the given transaction context.
// Every non-empty field must match for the route to match overall.
func (r Route) Matches(tx sdk.TransactionContext) bool {
	if r.VendorID != "" && !GlobMatch(r.VendorID, tx.VendorID, '/') {
		return false
	}
	if r.MarketplaceID != "" && !GlobMatch(r.MarketplaceID, tx.MarketplaceID, '/') {
		return false
	}
	if r.ProductID != "" && !GlobMatch(r.ProductID, tx.ProductID, '/') {
		return false
	}
	if r.EnvironmentID != "" && !GlobMatch(r.EnvironmentID, tx.EnvironmentID, '/') {
		return false
	}
	if r.TargetURL != "" && !matchTargetURL(r.TargetURL, tx.TargetURL) {
		return false
	}
	return true
}

// matchTargetURL strips the URL scheme before matching so patterns like
// "*.graph.microsoft.com/**" work against full URLs like
// "https://api.graph.microsoft.com/v1/users".
func matchTargetURL(pattern, targetURL string) bool {
	return GlobMatch(pattern, stripScheme(targetURL), '/')
}

// stripScheme removes the scheme prefix (e.g., "https://") from a URL.
func stripScheme(rawURL string) string {
	if _, after, ok := strings.Cut(rawURL, "://"); ok {
		return after
	}
	return rawURL
}
