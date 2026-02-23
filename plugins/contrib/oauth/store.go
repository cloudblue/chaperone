// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import "context"

// TokenStore abstracts refresh token persistence for a single session.
//
// A TokenStore is scoped to one token URL, one client, and one refresh token.
// It has no concept of tenants or resources — higher layers (e.g., the
// Microsoft building block) bridge a keyed store to this keyless interface
// via an adapter with pre-bound keys.
//
// Implementations must be safe for concurrent use and should be durable —
// a failed [TokenStore.Save] after a successful token exchange means the
// rotated refresh token is lost, since the old one has been invalidated.
type TokenStore interface {
	// Load retrieves the current refresh token.
	Load(ctx context.Context) (refreshToken string, err error)

	// Save persists a rotated refresh token after a successful exchange.
	Save(ctx context.Context, refreshToken string) error
}
