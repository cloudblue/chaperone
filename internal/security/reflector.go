// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"net/http"
	"strings"
)

// defaultSensitiveHeaders is the canonical list of headers that MUST be
// redacted in logs and stripped from responses. This is a security-critical
// default per Design Spec Section 5.3 and is the single source of truth used
// by both internal/config (for the merged sensitive_headers list applied to
// the vendor proxy path) and the forward proxy path (which uses these
// defaults verbatim).
//
// Do not duplicate this list elsewhere — consumers should call
// DefaultSensitiveHeaders or StripSensitiveResponseHeaders.
var defaultSensitiveHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Set-Cookie",
	"X-API-Key",
	"X-Auth-Token",
}

// defaultSensitiveHeadersSet is the lookup table built from
// defaultSensitiveHeaders, used by StripSensitiveResponseHeaders. Keys are
// stored lowercase for case-insensitive matching.
var defaultSensitiveHeadersSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(defaultSensitiveHeaders))
	for _, h := range defaultSensitiveHeaders {
		m[strings.ToLower(h)] = struct{}{}
	}
	return m
}()

// DefaultSensitiveHeaders returns a fresh copy of the built-in static list
// of sensitive headers. The caller is free to mutate the returned slice.
func DefaultSensitiveHeaders() []string {
	out := make([]string, len(defaultSensitiveHeaders))
	copy(out, defaultSensitiveHeaders)
	return out
}

// StripSensitiveResponseHeaders removes the built-in static set of sensitive
// headers (Authorization, Cookie, etc.) from headers in place. Matching is
// case-insensitive.
//
// This is the free-function counterpart to Reflector.StripResponseHeaders.
// Use it on code paths that do not have access to a configured Reflector
// (e.g., the forward proxy path), where the user-extended sensitive_headers
// list is intentionally not applied.
//
// Per Design Spec Section 5.3 "Credential Reflection Protection".
func StripSensitiveResponseHeaders(headers http.Header) {
	var toDelete []string
	for header := range headers {
		if _, ok := defaultSensitiveHeadersSet[strings.ToLower(header)]; ok {
			toDelete = append(toDelete, header)
		}
	}
	for _, header := range toDelete {
		headers.Del(header)
	}
}

// Reflector handles stripping sensitive headers from HTTP responses.
// Per Design Spec Section 5.3 "Credential Reflection Protection":
// "The Proxy strips all Injection Headers (like Authorization) from the
// Response before sending it back to Connect."
type Reflector struct {
	sensitiveHeaders map[string]struct{}
}

// NewReflector creates a new Reflector with the given list of sensitive headers.
// Headers are matched case-insensitively.
func NewReflector(sensitiveHeaders []string) *Reflector {
	s := &Reflector{
		sensitiveHeaders: make(map[string]struct{}, len(sensitiveHeaders)),
	}
	for _, h := range sensitiveHeaders {
		// Store lowercase for case-insensitive matching
		s.sensitiveHeaders[strings.ToLower(h)] = struct{}{}
	}
	return s
}

// ShouldStrip returns true if the header should be stripped from responses.
// Matching is case-insensitive.
func (s *Reflector) ShouldStrip(header string) bool {
	_, ok := s.sensitiveHeaders[strings.ToLower(header)]
	return ok
}

// StripResponseHeaders removes sensitive headers from the response headers.
// This modifies the headers in place.
//
// Per Design Spec Section 5.3: "The Core runs the Response Sanitizer, which
// unconditionally strips dangerous headers (e.g., Authorization) before the
// response is returned to Connect or logged."
func (s *Reflector) StripResponseHeaders(headers http.Header) {
	var toDelete []string
	for header := range headers {
		if s.ShouldStrip(header) {
			toDelete = append(toDelete, header)
		}
	}
	for _, header := range toDelete {
		headers.Del(header)
	}
}

// injectedHeadersKey is an unexported type for the context key that stores
// dynamically injected header names, preventing collisions with other packages.
type injectedHeadersKey struct{}

// WithInjectedHeaders stores the header keys that the plugin injected into
// the outgoing request. The Reflector uses these to strip the same headers
// from the ISV's response, preventing credential reflection even for
// non-standard header names not in the static sensitive list.
//
// Called from injectCredentials after the plugin's GetCredentials returns.
func WithInjectedHeaders(ctx context.Context, keys []string) context.Context {
	return context.WithValue(ctx, injectedHeadersKey{}, keys)
}

// InjectedHeaders retrieves the list of injected header keys from context.
// Returns nil if no injected headers have been stored.
func InjectedHeaders(ctx context.Context) []string {
	keys, _ := ctx.Value(injectedHeadersKey{}).([]string)
	return keys
}

// StripInjectedHeaders removes dynamically injected headers from the response.
// This complements StripResponseHeaders (which handles the static configured
// list) by also stripping whatever headers the plugin actually injected
// into this specific request.
//
// Per Design Spec Section 5.3 "Credential Reflection Protection":
// "The Proxy strips all Injection Headers" — this means both the well-known
// static list AND whatever was injected per-request.
//
// Uses http.Header.Del which is case-insensitive.
func StripInjectedHeaders(ctx context.Context, headers http.Header) {
	for _, key := range InjectedHeaders(ctx) {
		headers.Del(key)
	}
}
