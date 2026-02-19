// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package sdk

// TransactionContext contains the metadata for a single proxy request.
//
// This information is extracted from the inbound request headers
// (using the configured header prefix, default "X-Connect-") and
// passed to the plugin for credential resolution.
//
// The context is also used to compute the cache key for Fast Path caching.
type TransactionContext struct {
	// Data contains additional context from {HeaderPrefix}-Context-Data.
	// This is a Base64-encoded JSON object in the header, unmarshaled here.
	// Use this for ISV-specific parameters that don't fit standard fields.
	Data map[string]any
	// TraceID is the correlation ID for distributed tracing.
	// Extracted from the configured trace header (default: "Connect-Request-ID").
	// If not present in the request, a UUID is generated.
	TraceID string
	// EnvironmentID identifies the runtime environment (e.g., "production", "test").
	// Extracted from {HeaderPrefix}-Environment-ID.
	EnvironmentID string
	// MarketplaceID identifies the marketplace (e.g., "US", "EU").
	// Extracted from {HeaderPrefix}-Marketplace-ID.
	MarketplaceID string
	// VendorID identifies the vendor account owning the product.
	// Extracted from {HeaderPrefix}-Vendor-ID.
	VendorID string
	// ProductID identifies the specific product SKU.
	// Extracted from {HeaderPrefix}-Product-ID.
	ProductID string
	// SubscriptionID is the unique subscription identifier.
	// Extracted from {HeaderPrefix}-Subscription-ID.
	SubscriptionID string
	// TargetURL is the full destination URL for this request.
	// Extracted from {HeaderPrefix}-Target-URL.
	// This has already been validated against the allow-list by the core.
	TargetURL string
}
