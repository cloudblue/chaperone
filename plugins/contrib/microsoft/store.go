// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package microsoft

import "context"

// TokenStore abstracts multi-tenant refresh token persistence for the
// Microsoft Secure Application Model.
//
// Unlike [oauth.TokenStore], which is keyless and scoped to a single session,
// this interface is keyed by tenantID only — reflecting the fact that Azure AD
// refresh tokens are Multi-Resource Refresh Tokens (MRRTs). A single refresh
// token per tenant can be exchanged for access tokens to any consented
// resource.
//
// Implementations must be safe for concurrent use and should be durable —
// a failed [TokenStore.Save] after a successful token exchange means the
// rotated refresh token may be lost if the old one has been invalidated.
type TokenStore interface {
	// Load retrieves the current refresh token for the given tenant.
	// Returns [contrib.ErrTenantNotFound] if no refresh token exists for this
	// tenant.
	Load(ctx context.Context, tenantID string) (refreshToken string, err error)

	// Save persists a rotated refresh token after a successful exchange.
	Save(ctx context.Context, tenantID string, refreshToken string) error
}
