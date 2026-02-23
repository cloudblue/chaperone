// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package microsoft

import "context"

// TokenStore abstracts multi-tenant refresh token persistence for the
// Microsoft Secure Application Model.
//
// Unlike [oauth.TokenStore], which is keyless and scoped to a single session,
// this interface is keyed by tenantID and resource — reflecting the fact that
// a single Microsoft app registration is shared across many tenants, each
// with its own refresh token per resource.
//
// Implementations must be safe for concurrent use and should be durable —
// a failed [TokenStore.Save] after a successful token exchange means the
// rotated refresh token is lost, since the old one has been invalidated by
// Microsoft's token endpoint.
type TokenStore interface {
	// Load retrieves the current refresh token for the given tenant and resource.
	// Returns [contrib.ErrTenantNotFound] if no refresh token exists for this
	// tenant+resource pair.
	Load(ctx context.Context, tenantID, resource string) (refreshToken string, err error)

	// Save persists a rotated refresh token after a successful exchange.
	Save(ctx context.Context, tenantID, resource string, refreshToken string) error
}
