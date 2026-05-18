// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import "github.com/cloudblue/chaperone/sdk"

// Action is the sealed interface implemented by [CredentialAction] and
// [ForwardAction]. The unexported isAction method prevents implementations
// outside this package, so the mux can exhaustively reason about the two
// dispatch outcomes: credential injection vs. raw forwarding.
type Action interface {
	isAction()
}

// CredentialAction routes a matched request to a [sdk.CredentialProvider]
// for normal credential injection. This is the default action installed
// by [Mux.Handle].
type CredentialAction struct {
	Provider sdk.CredentialProvider
}

func (CredentialAction) isAction() {}

// ForwardAction routes a matched request to a named forward_target. The
// Mux returns a [sdk.RouteAction] with ForwardTo set to Target from
// RouteRequest, and the Core handles the actual forwarding (the request
// never reaches a credential provider).
//
// Target validation (non-empty, references an existing forward_target)
// happens at config-load / cross-validation time, not here.
type ForwardAction struct {
	Target string
}

func (ForwardAction) isAction() {}
