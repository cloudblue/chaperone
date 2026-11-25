// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauthutil

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

// RejectionCooldown is the minimum interval between two discards of a cached
// access token caused by the target rejecting it. It bounds the token
// exchanges a target can force to one per cache key per interval: a target
// that answers 401 for a reason a new token does not fix (missing permission,
// wrong audience, a disabled account) must not turn every request into a
// call to the token endpoint.
const RejectionCooldown = 1 * time.Minute

// bearerPrefix is the Authorization scheme prefix the building blocks inject.
const bearerPrefix = "Bearer "

// RejectedBearerToken returns the bearer token that a request was sent with
// when the target answered it with 401 Unauthorized. ok is false for any
// other status, and when the request carried no bearer token.
//
// resp.Request is the request as it was sent to the target, so its
// Authorization header is the one the credential provider injected.
func RejectedBearerToken(resp *http.Response) (token string, ok bool) {
	if resp == nil || resp.StatusCode != http.StatusUnauthorized || resp.Request == nil {
		return "", false
	}
	token, ok = strings.CutPrefix(resp.Request.Header.Get("Authorization"), bearerPrefix)
	if !ok || token == "" {
		return "", false
	}
	return token, true
}

// SameToken reports whether two tokens are equal, in constant time.
func SameToken(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// InCooldown reports whether a discard at now falls within RejectionCooldown
// of the previous discard. A zero last means there was no previous discard.
func InCooldown(last, now time.Time) bool {
	return !last.IsZero() && now.Sub(last) < RejectionCooldown
}
