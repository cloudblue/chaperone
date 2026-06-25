// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package compliance_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
	"github.com/cloudblue/chaperone/sdk/compliance"
)

// stubRouter implements sdk.RequestRouter for the compliance test.
type stubRouter struct {
	action *sdk.RouteAction
	err    error
}

func (s *stubRouter) RouteRequest(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.RouteAction, error) {
	return s.action, s.err
}

func TestVerifyRouter_NilActionAndNilError_Passes(t *testing.T) {
	compliance.VerifyRouter(t, &stubRouter{})
}

func TestVerifyRouter_NonEmptyForwardTo_Passes(t *testing.T) {
	compliance.VerifyRouter(t, &stubRouter{action: &sdk.RouteAction{ForwardTo: "x"}})
}
