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

// ForwardProxyForTesting returns the *ForwardProxy registered under the given
// name, or nil if no such target was configured. Exposed for external tests
// that verify the per-target forward registry built at startup.
func (s *Server) ForwardProxyForTesting(name string) *ForwardProxy {
	return s.forwardProxies[name]
}

// ForwardProxyCountForTesting returns the number of forward proxies built at
// startup. Exposed for external tests that verify the registry is non-nil
// even when no forward targets are configured.
func (s *Server) ForwardProxyCountForTesting() int {
	return len(s.forwardProxies)
}

// ForwardProxiesNilForTesting reports whether the forward proxy map is nil.
// Exposed so external tests can assert the registry is non-nil (the spec
// requires an empty map, not nil) even with zero configured targets.
func (s *Server) ForwardProxiesNilForTesting() bool {
	return s.forwardProxies == nil
}

// RouterForTesting returns the RequestRouter detected on the plugin at startup,
// or nil if the plugin does not implement RequestRouter. Exposed for external
// tests that verify RequestRouter type assertion and capability detection.
// This must remain unexported for proxy package but tests access via this method.
func (s *Server) RouterForTesting() interface{} {
	return s.router
}
