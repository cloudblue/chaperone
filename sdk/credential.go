// Copyright 2024-2026 CloudBlue
// SPDX-License-Identifier: Apache-2.0

package sdk

import "time"

// Credential represents authentication data to be injected into a request.
//
// When returned from GetCredentials, the proxy core will:
//  1. Inject all Headers into the outgoing request
//  2. Cache this credential using the TransactionContext hash as key
//  3. Serve subsequent matching requests from cache until ExpiresAt
//
// For credentials that cannot be cached (e.g., HMAC signatures that depend
// on request body), return nil from GetCredentials instead and mutate
// the request directly.
type Credential struct {
	// Headers contains the authentication headers to inject.
	// Common examples:
	//   - "Authorization": "Bearer <token>"
	//   - "X-API-Key": "<key>"
	//   - "Cookie": "<session>"
	Headers map[string]string

	// ExpiresAt determines when this cached credential becomes invalid.
	// The proxy will call GetCredentials again after this time.
	//
	// Best practices:
	//   - Set slightly before actual token expiry (e.g., token expires in 1h, set 55m)
	//   - For non-expiring API keys, use a reasonable refresh interval (e.g., 24h)
	//   - Never set to zero; use a minimum of 1 minute
	ExpiresAt time.Time
}

// IsExpired returns true if the credential has passed its expiration time.
func (c *Credential) IsExpired() bool {
	if c == nil {
		return true
	}
	return time.Now().After(c.ExpiresAt)
}

// TTL returns the remaining time until the credential expires.
// Returns 0 if already expired.
func (c *Credential) TTL() time.Duration {
	if c == nil || c.IsExpired() {
		return 0
	}
	return time.Until(c.ExpiresAt)
}
