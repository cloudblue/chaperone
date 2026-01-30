// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"errors"
	"net/url"
)

// allowInsecureTargets controls whether HTTP (non-HTTPS) target URLs are permitted.
// This is set at compile time via ldflags. Default is "false" (secure).
//
// SECURITY: In production builds, this MUST be "false" to prevent credentials
// from being sent over unencrypted connections.
//
// Set via: -ldflags "-X 'github.com/cloudblue/chaperone/internal/proxy.allowInsecureTargets=true'"
var allowInsecureTargets = "false"

// testOverrideInsecureTargets is used by tests to temporarily allow HTTP targets.
// This is NOT settable via ldflags - only via SetAllowInsecureTargetsForTesting.
var testOverrideInsecureTargets *bool

// AllowInsecureTargets returns true if HTTP targets are permitted.
// This should only be true in development builds.
func AllowInsecureTargets() bool {
	// Test override takes precedence
	if testOverrideInsecureTargets != nil {
		return *testOverrideInsecureTargets
	}
	return allowInsecureTargets == "true"
}

// SetAllowInsecureTargetsForTesting allows tests to temporarily enable HTTP targets.
// Returns a cleanup function that restores the original value.
// This function panics if called from non-test code (no _test.go in stack).
func SetAllowInsecureTargetsForTesting(allow bool) func() {
	old := testOverrideInsecureTargets
	testOverrideInsecureTargets = &allow
	return func() {
		testOverrideInsecureTargets = old
	}
}

// ErrInsecureTargetURL is returned when an HTTP target URL is used in production mode.
var ErrInsecureTargetURL = errors.New("HTTPS required: insecure HTTP target URLs are not allowed in production builds")

// ValidateTargetScheme checks that the target URL uses HTTPS.
// Returns an error if the scheme is HTTP and insecure targets are not allowed.
func ValidateTargetScheme(target *url.URL) error {
	if target == nil {
		return errors.New("target URL is nil")
	}

	if target.Scheme == "https" {
		return nil
	}

	if target.Scheme == "http" {
		if AllowInsecureTargets() {
			// Development mode: allow but warn
			return nil
		}
		return ErrInsecureTargetURL
	}

	// Unknown scheme
	return errors.New("unsupported URL scheme: " + target.Scheme)
}
