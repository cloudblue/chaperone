// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import "net/http"

// WithMiddlewareForTesting exposes the unexported withMiddleware method
// for external test packages. This enables structural tests that verify
// the global middleware chain includes PanicRecoveryMiddleware.
func (s *Server) WithMiddlewareForTesting(handler http.Handler) http.Handler {
	return s.withMiddleware(handler)
}
