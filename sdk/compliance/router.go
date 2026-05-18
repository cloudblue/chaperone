// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package compliance

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

// VerifyRouter exercises a sdk.RequestRouter implementation against the
// minimal contract: it must accept a cancelled context without panicking
// and return either (nil, nil) (fall-through) or a non-nil RouteAction.
//
// VerifyRouter is opt-in: only plugins that implement RequestRouter need
// to call it. Plugins that do not implement RequestRouter remain valid
// under VerifyContract.
func VerifyRouter(t *testing.T, router sdk.RequestRouter) {
	t.Helper()

	t.Run("returns without panicking on cancelled context", func(t *testing.T) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", http.NoBody)
		_, _ = router.RouteRequest(ctx, sdk.TransactionContext{}, req) // no panic = pass
	})

	t.Run("nil RouteAction is a valid fall-through signal", func(t *testing.T) {
		t.Helper()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test", http.NoBody)
		action, err := router.RouteRequest(context.Background(), sdk.TransactionContext{}, req)
		if err != nil {
			return // returning an error is also valid
		}
		if action != nil && action.ForwardTo == "" {
			t.Fatalf("router returned non-nil RouteAction with empty ForwardTo; use nil instead")
		}
	})
}
