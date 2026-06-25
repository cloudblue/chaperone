// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
)

// allowInsecureForwardTargets controls whether HTTP (non-HTTPS) forward
// target URLs are permitted. This is set at compile time via ldflags;
// the default is "false" (secure).
//
// SECURITY: In production builds, this MUST be "false" to prevent the
// bearer token (or any other credential) from being sent over an
// unencrypted connection.
//
// Set via: -ldflags "-X 'github.com/cloudblue/chaperone/internal/config.allowInsecureForwardTargets=true'"
//
// The matching dev-build variable for vendor targets lives in
// internal/proxy.allowInsecureTargets; we mirror the same pattern here
// rather than reach across packages because internal/proxy already
// imports internal/config (so we cannot import the other direction).
var allowInsecureForwardTargets = "false"

// testOverrideInsecureForwardTargets is used by tests to temporarily
// allow http forward targets. It is always nil unless set by test code.
var testOverrideInsecureForwardTargets *bool

// AllowInsecureForwardTargets reports whether http forward target URLs
// are permitted. This is true only in dev builds or under an explicit
// test override.
func AllowInsecureForwardTargets() bool {
	if testOverrideInsecureForwardTargets != nil {
		return *testOverrideInsecureForwardTargets
	}
	return allowInsecureForwardTargets == "true"
}

// SetAllowInsecureForwardTargetsForTesting temporarily enables http
// forward target URLs. It returns a cleanup function that restores the
// previous value. Intended for tests only.
func SetAllowInsecureForwardTargetsForTesting(allow bool) func() {
	old := testOverrideInsecureForwardTargets
	testOverrideInsecureForwardTargets = &allow
	return func() {
		testOverrideInsecureForwardTargets = old
	}
}

// Forward target auth types.
const (
	// ForwardAuthNone disables authentication on the forward target.
	ForwardAuthNone = "none"
	// ForwardAuthBearer attaches a static bearer token on the forward target.
	ForwardAuthBearer = "bearer"
)

// Forward target validation errors. They are exported so that callers
// (and tests) can match on them with errors.Is.
var (
	// ErrForwardTargetMissingURL is returned when a forward target has no url.
	ErrForwardTargetMissingURL = errors.New("forward target: url is required")
	// ErrForwardTargetInvalidURL is returned when a forward target url cannot be parsed.
	ErrForwardTargetInvalidURL = errors.New("forward target: invalid url")
	// ErrForwardTargetInsecureURL is returned when a forward target url is not https
	// (and http is not allowed in this build).
	ErrForwardTargetInsecureURL = errors.New("forward target: url must be https")
	// ErrForwardTargetAuthTypeMissing is returned when auth.type is unset.
	ErrForwardTargetAuthTypeMissing = errors.New("forward target: auth.type is required")
	// ErrForwardTargetAuthTypeUnsupported is returned when auth.type is not one of the supported values.
	ErrForwardTargetAuthTypeUnsupported = errors.New("forward target: unsupported auth.type")
	// ErrForwardTargetBearerTokenMissing is returned when bearer auth is configured without a token.
	ErrForwardTargetBearerTokenMissing = errors.New("forward target: bearer auth requires a non-empty token")
)

// interpolateForwardTargetEnv expands environment variable references in
// the credential-bearing fields of every forward target. Only fields
// listed here are interpolated, so the rest of the config behaves
// exactly like before. Uses os.ExpandEnv semantics (${VAR} and $VAR).
func interpolateForwardTargetEnv(cfg *Config) {
	if cfg == nil {
		return
	}
	for name, t := range cfg.ForwardTargets {
		t.Auth.Token = os.ExpandEnv(t.Auth.Token)
		cfg.ForwardTargets[name] = t
	}
}

// validateForwardTargets validates every entry in cfg.ForwardTargets.
// When allowHTTP is true (dev builds), http urls are permitted; otherwise
// only https is accepted. Errors include the offending target name so
// operators can locate the misconfiguration quickly.
func validateForwardTargets(cfg *Config, allowHTTP bool) error {
	var errs []error
	for name, t := range cfg.ForwardTargets {
		if err := validateForwardTarget(name, t, allowHTTP); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// validateForwardTarget validates a single forward target entry. Split
// out from validateForwardTargets to keep cognitive complexity in check.
func validateForwardTarget(name string, t ForwardTargetConfig, allowHTTP bool) error {
	if t.URL == "" {
		return fmt.Errorf("forward_targets[%q]: %w", name, ErrForwardTargetMissingURL)
	}
	u, err := url.Parse(t.URL)
	if err != nil {
		return fmt.Errorf("forward_targets[%q]: %w: %w", name, ErrForwardTargetInvalidURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("forward_targets[%q]: %w: %q", name, ErrForwardTargetInvalidURL, t.URL)
	}
	if u.Scheme != "https" && (!allowHTTP || u.Scheme != "http") {
		return fmt.Errorf("forward_targets[%q]: %w (got %q)", name, ErrForwardTargetInsecureURL, u.Scheme)
	}
	return validateForwardTargetAuth(name, t.Auth)
}

// validateForwardTargetAuth validates the auth subsection of a forward target.
func validateForwardTargetAuth(name string, auth ForwardTargetAuthConfig) error {
	switch auth.Type {
	case ForwardAuthNone:
		return nil
	case ForwardAuthBearer:
		if auth.Token == "" {
			return fmt.Errorf("forward_targets[%q]: %w", name, ErrForwardTargetBearerTokenMissing)
		}
		return nil
	case "":
		return fmt.Errorf("forward_targets[%q]: %w (expected %q or %q)",
			name, ErrForwardTargetAuthTypeMissing, ForwardAuthBearer, ForwardAuthNone)
	default:
		return fmt.Errorf("forward_targets[%q]: %w: %q", name, ErrForwardTargetAuthTypeUnsupported, auth.Type)
	}
}
