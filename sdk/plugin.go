// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package sdk defines the public interfaces that Distributors implement
// to create custom plugins for the Chaperone egress proxy.
//
// This module is versioned independently from the core proxy to ensure
// that Distributors can safely upgrade the proxy without modifying their
// plugin code, as long as the SDK major version remains stable.
package sdk

import (
	"context"
	"net/http"
)

// Plugin is the main interface that Distributors implement to inject
// custom logic into the Chaperone proxy.
//
// A Plugin must implement all three sub-interfaces:
//   - CredentialProvider: Injects authentication credentials into requests
//   - CertificateSigner: Signs CSRs for automatic certificate rotation
//   - ResponseModifier: Optionally modifies responses before returning to upstream
type Plugin interface {
	CredentialProvider
	CertificateSigner
	ResponseModifier
}

// CredentialProvider handles credential injection for outgoing requests.
//
// Implementations can choose between two strategies:
//
// Fast Path (Caching): Return a *Credential with headers and TTL.
// The proxy will cache this and skip plugin execution for subsequent
// requests with the same context hash.
//
// Slow Path (Direct Mutation): Mutate the request directly and return (nil, nil).
// Use this for complex auth schemes like HMAC body signing where the
// credential depends on request content. The plugin runs on every request.
//
// # Context Usage
//
// The ctx parameter is bounded by the Core with a request timeout and is
// cancelled if the upstream client disconnects. Implementations that make
// network calls (HTTP, database, Vault) SHOULD respect this context to
// allow prompt cancellation and avoid resource leaks.
//
// For simple implementations (like file-based credential lookup), context
// checking is optional since operations complete in microseconds.
//
// Example (network call respecting context):
//
//	func (p *MyPlugin) GetCredentials(ctx context.Context, tx TransactionContext, req *http.Request) (*Credential, error) {
//	    // Create HTTP request with context - cancellation handled automatically
//	    vaultReq, _ := http.NewRequestWithContext(ctx, "GET", p.vaultURL, nil)
//	    resp, err := p.httpClient.Do(vaultReq)
//	    if err != nil {
//	        return nil, fmt.Errorf("vault request failed: %w", err) // includes context.Canceled/DeadlineExceeded
//	    }
//	    // ... process response
//	}
type CredentialProvider interface {
	// GetCredentials retrieves or generates credentials for the given transaction.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - tx: Transaction context with vendor, product, and subscription info
	//   - req: The outgoing HTTP request (can be mutated for Slow Path)
	//
	// Returns:
	//   - *Credential: Headers to inject + TTL (Fast Path), or nil (Slow Path)
	//   - error: Any error during credential retrieval
	//
	// Fast Path Example:
	//   return &Credential{
	//       Headers: map[string]string{"Authorization": "Bearer " + token},
	//       ExpiresAt: time.Now().Add(1 * time.Hour),
	//   }, nil
	//
	// Slow Path Example (body signing):
	//   signature := sign(req.Body)
	//   req.Header.Set("X-Signature", signature)
	//   return nil, nil
	GetCredentials(ctx context.Context, tx TransactionContext, req *http.Request) (*Credential, error)
}

// CertificateSigner handles certificate signing for automatic rotation.
//
// When the proxy's TLS certificate approaches expiration, the core generates
// a new key pair and CSR, then calls SignCSR to obtain a signed certificate.
type CertificateSigner interface {
	// SignCSR signs a Certificate Signing Request using the configured CA.
	//
	// The implementation should forward the CSR to the appropriate CA
	// (Connect API, HashiCorp Vault, internal PKI, etc.) and return
	// the signed certificate.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - csrPEM: PEM-encoded Certificate Signing Request
	//
	// Returns:
	//   - []byte: PEM-encoded signed certificate
	//   - error: Any error during signing
	SignCSR(ctx context.Context, csrPEM []byte) (crtPEM []byte, err error)
}

// ResponseAction tells the Core how to handle the response after plugin processing.
// Return nil from ModifyResponse for default behavior (Core applies safety net).
type ResponseAction struct {
	// SkipErrorNormalization prevents Core from sanitizing error responses (4xx/5xx).
	// When true, the response body is passed through as-is to the upstream platform.
	//
	// Use this when:
	//   - The ISV returns structured validation errors that Connect needs
	//   - The plugin has already sanitized/customized the error response
	//
	// The Core will still:
	//   - Strip sensitive headers (Authorization, etc.)
	//   - Add X-Error-ID for correlation
	SkipErrorNormalization bool
}

// ResponseModifier allows post-processing of responses before returning
// them to the upstream platform.
//
// Use cases include:
//   - Stripping PII or internal headers from vendor responses
//   - Normalizing error codes across different vendors
//   - Logging specific response fields for debugging
//   - Passing through ISV validation errors to Connect
//
// Note: Reading resp.Body will buffer the entire response into memory,
// which may impact performance for large responses.
type ResponseModifier interface {
	// ModifyResponse processes the response before it's returned upstream.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - tx: Transaction context for this request
	//   - resp: The HTTP response (can be modified in place)
	//
	// Returns:
	//   - *ResponseAction: Instructions for Core, or nil for default behavior
	//     (nil means Core applies error normalization as safety net)
	//   - error: Any error during modification (will be logged, response still sent)
	ModifyResponse(ctx context.Context, tx TransactionContext, resp *http.Response) (*ResponseAction, error)
}
