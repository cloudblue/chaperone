// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package sdk

import (
	"context"
	"net/http"
)

// RequestRouter is an optional plugin capability that decides, per request,
// whether to forward the request to a different upstream instead of
// proceeding with credential injection and the vendor call.
//
// Plugins that do not implement RequestRouter retain today's behavior.
type RequestRouter interface {
	// RouteRequest is invoked before GetCredentials. Returning a non-nil
	// RouteAction with a non-empty ForwardTo causes the Core to forward
	// the request to the named forward_target and skip both credential
	// injection and ModifyResponse.
	//
	// Returning nil (or an empty ForwardTo) is the fall-through signal:
	// the Core continues with the normal credential-injection flow.
	RouteRequest(ctx context.Context, tx TransactionContext, req *http.Request) (*RouteAction, error)
}

// RouteAction signals how the Core should handle this request.
type RouteAction struct {
	// ForwardTo names a forward_target defined in the proxy configuration.
	// When non-empty, the Core forwards the request to that target's URL
	// and skips credential injection and ModifyResponse.
	ForwardTo string
}
