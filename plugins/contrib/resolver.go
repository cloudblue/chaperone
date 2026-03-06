// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"fmt"

	"github.com/cloudblue/chaperone/sdk"
)

// KeyResolver maps a transaction context to a credential key.
//
// Providers call ResolveKey when the request does not carry an explicit
// override in tx.Data. The returned key identifies which stored credential
// to use (e.g., a tenant ID, a session name, an account identifier).
//
// A successful return must be a non-empty string. Returns
// ErrMissingContextData if the transaction lacks enough information
// to resolve a key.
type KeyResolver interface {
	ResolveKey(ctx context.Context, tx sdk.TransactionContext) (string, error)
}

// ResolveFromContext extracts a key from tx.Data if present, otherwise
// delegates to the resolver. Returns ErrMissingContextData if neither
// source provides a value.
//
// Malformed overrides (present but wrong type or empty string) always
// return ErrInvalidContextData — they never fall through to the resolver.
// This preserves strictness: a connector bug that sends a bad value is
// surfaced immediately, not silently masked by the resolver.
func ResolveFromContext(
	ctx context.Context,
	tx sdk.TransactionContext,
	dataField string,
	resolver KeyResolver,
) (string, error) {
	raw, ok := tx.Data[dataField]
	if ok {
		return extractOverride(raw, dataField)
	}

	if resolver != nil {
		key, err := resolver.ResolveKey(ctx, tx)
		if err != nil {
			return "", fmt.Errorf("resolving %s: %w", dataField, err)
		}
		if key == "" {
			return "", fmt.Errorf("resolver returned empty %s: %w",
				dataField, ErrMissingContextData)
		}
		return key, nil
	}

	return "", fmt.Errorf("%s not present in transaction context: %w",
		dataField, ErrMissingContextData)
}

// extractOverride validates an explicit override from tx.Data. It returns
// ErrInvalidContextData for wrong type or empty string — these never fall
// through to the resolver.
func extractOverride(raw any, field string) (string, error) {
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %T: %w",
			field, raw, ErrInvalidContextData)
	}

	if s == "" {
		return "", fmt.Errorf("%s is empty in transaction context: %w",
			field, ErrInvalidContextData)
	}

	return s, nil
}
