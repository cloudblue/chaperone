// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"net/http"

	"github.com/cloudblue/chaperone/sdk"
)

// RejectedCredentialHandler is an optional interface for credential providers
// that keep credentials in memory, such as the OAuth2 building blocks.
//
// A cached access token can stop working before its expiry time: the identity
// provider revoked it (password change, session revocation, policy change),
// or the local clock is wrong. The only signal is the target answering
// 401 Unauthorized. A provider that implements this interface discards the
// credential that was rejected, so the next GetCredentials call obtains a
// new one instead of repeating the rejected one until it expires.
//
// [Mux] and [AsPlugin] call CredentialRejected from ModifyResponse for the
// provider that serves the request. A Distributor plugin that wraps a
// building block in its own [sdk.Plugin] must forward the call from its own
// ModifyResponse.
type RejectedCredentialHandler interface {
	// CredentialRejected is called when the target answered a request with
	// 401 Unauthorized. resp.Request is the request as it was sent to the
	// target, including the credential headers that were injected.
	//
	// Implementations must only discard a cached credential when it is the
	// one the request was sent with, must bound how often they discard (a
	// target can answer 401 for reasons a new credential does not fix), and
	// must not read resp.Body. The request is not retried: the 401 still
	// reaches the caller, and the next request uses a new credential.
	CredentialRejected(ctx context.Context, tx sdk.TransactionContext, resp *http.Response)
}

// notifyCredentialRejected tells the provider that the target rejected the
// credential of this request, when the response is a 401 and the provider
// implements [RejectedCredentialHandler].
func notifyCredentialRejected(ctx context.Context, provider sdk.CredentialProvider, tx sdk.TransactionContext, resp *http.Response) {
	if provider == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return
	}
	if h, ok := provider.(RejectedCredentialHandler); ok {
		h.CredentialRejected(ctx, tx, resp)
	}
}
