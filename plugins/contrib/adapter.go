// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"net/http"

	"github.com/cloudblue/chaperone/sdk"
)

// Compile-time check that pluginAdapter implements sdk.Plugin.
var _ sdk.Plugin = (*pluginAdapter)(nil)

// AsPlugin wraps a CredentialProvider into a full sdk.Plugin.
//
// This adapter provides minimal implementations for SignCSR and ModifyResponse,
// matching the reference plugin's behavior:
//   - SignCSR returns an error (signing requires explicit configuration)
//   - ModifyResponse returns nil (safe default, lets Core handle normalization).
//     It leaves the response untouched; on a 401 from the target it notifies
//     a provider that implements [RejectedCredentialHandler].
//
// Use this to pass a building block (e.g., oauth.ClientCredentials) to the
// SDK compliance test kit:
//
//	provider := oauth.NewClientCredentials(cfg)
//	compliance.VerifyContract(t, contrib.AsPlugin(provider))
func AsPlugin(provider sdk.CredentialProvider) sdk.Plugin {
	return &pluginAdapter{provider: provider}
}

type pluginAdapter struct {
	provider sdk.CredentialProvider
}

func (a *pluginAdapter) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
	return a.provider.GetCredentials(ctx, tx, req)
}

func (a *pluginAdapter) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
	return nil, ErrSigningNotConfigured
}

func (a *pluginAdapter) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) (*sdk.ResponseAction, error) {
	notifyCredentialRejected(ctx, a.provider, tx, resp)
	return nil, nil
}
