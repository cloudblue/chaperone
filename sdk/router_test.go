// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package sdk

import "testing"

func TestRouteAction_ZeroValue_IsNonForwarding(t *testing.T) {
	var a RouteAction
	if a.ForwardTo != "" {
		t.Fatalf("zero RouteAction.ForwardTo = %q, want empty", a.ForwardTo)
	}
}

func TestRouteAction_WithForwardTo_PreservesName(t *testing.T) {
	a := RouteAction{ForwardTo: "company-b"}
	if a.ForwardTo != "company-b" {
		t.Fatalf("RouteAction.ForwardTo = %q, want %q", a.ForwardTo, "company-b")
	}
}
