// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package contrib provides reusable auth flow building blocks and a request
// multiplexer for the Chaperone egress proxy.
//
// Two layers:
//   - Building blocks (oauth.ClientCredentials, microsoft.TokenSource) implement
//     sdk.CredentialProvider and are composable inside distributor plugins.
//   - Mux routes requests to the right building block by VendorID, TargetURL,
//     or EnvironmentID. It implements full sdk.Plugin and can be passed directly
//     to chaperone.Run().
package contrib

import "errors"

// ErrNoRouteMatch indicates no mux route matched the request and no
// default handler is configured. This is a proxy configuration issue.
var ErrNoRouteMatch = errors.New("no route matched")

// ErrMissingContextData indicates required keys (TenantID, Resource) are
// not present in TransactionContext.Data. This is a platform/caller issue —
// the Connect platform sent a request without the expected context headers.
var ErrMissingContextData = errors.New("missing required context data")

// ErrInvalidContextData indicates a required key is present in
// TransactionContext.Data but has the wrong type (e.g., TenantID is a
// number instead of a string). Since Data is map[string]any from JSON
// unmarshaling, type assertions can fail. This is a platform/caller issue.
var ErrInvalidContextData = errors.New("invalid context data type")

// ErrTenantNotFound indicates the requested tenant is not in the static
// config and no resolver callback is set, or the resolver returned not found.
// This is a proxy configuration issue.
var ErrTenantNotFound = errors.New("tenant not found")

// ErrInvalidCredentials indicates the OAuth token endpoint rejected the
// client credentials (HTTP 401). Retrying won't help — the client secret
// is wrong or expired.
var ErrInvalidCredentials = errors.New("invalid client credentials")

// ErrTokenExpiredOnArrival indicates the token endpoint returned a token
// with expires_in <= ExpiryMargin. The token is too short-lived to cache.
var ErrTokenExpiredOnArrival = errors.New("token expired on arrival")

// ErrTokenEndpointUnavailable indicates a transient failure reaching the
// token endpoint (network error, HTTP 5xx, HTTP 429). Retrying may help.
var ErrTokenEndpointUnavailable = errors.New("token endpoint unavailable")

// ErrSigningNotConfigured indicates no certificate signer has been set.
// This is returned by adapters (AsPlugin, Mux) when SignCSR is called
// without a configured signer.
var ErrSigningNotConfigured = errors.New("certificate signing not configured")
