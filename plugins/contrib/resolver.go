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

// ResolveCredentialKey extracts a key from tx.Data if present, otherwise
// delegates to the resolver. Returns ErrMissingContextData if neither
// source provides a value.
//
// Malformed overrides (present but wrong type or empty string) always
// return ErrInvalidContextData — they never fall through to the resolver.
// This preserves strictness: a connector bug that sends a bad value is
// surfaced immediately, not silently masked by the resolver.
func ResolveCredentialKey(
	ctx context.Context,
	tx sdk.TransactionContext,
	dataField string,
	resolver KeyResolver,
) (string, error) {
	key, ok, err := tx.DataString(dataField)
	if err != nil {
		return "", err
	}
	if ok {
		return key, nil
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
