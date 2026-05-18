// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"fmt"

	"github.com/cloudblue/chaperone/sdk"
)

// MuxConfig is the YAML-friendly description of a request multiplexer.
// It can be parsed directly from a YAML document (or constructed in code)
// and passed to [LoadMuxFromConfig] to build a usable [*Mux].
//
// Mutual exclusion: every route — and the fallback, if present — must set
// exactly one of `forward` or `credentials`. See [LoadMuxFromConfig] for the
// validation rules.
type MuxConfig struct {
	// Routes are evaluated by specificity at dispatch time; registration
	// order is preserved and breaks ties.
	Routes []MuxRouteConfig `yaml:"routes"`
	// Fallback is the catch-all used when no route matches. Optional.
	Fallback *MuxFallbackConfig `yaml:"fallback,omitempty"`
}

// MuxRouteConfig is a single route entry in a YAML mux configuration.
// Exactly one of Forward or Credentials must be set.
type MuxRouteConfig struct {
	// Match contains the route's matching criteria. Empty fields are
	// wildcards. See [Route] for semantics.
	Match MatchConfig `yaml:"match"`
	// Forward names a forward_target. When set, the matched request is
	// forwarded as-is to that target by the Core. Mutually exclusive with
	// Credentials.
	Forward string `yaml:"forward,omitempty"`
	// Credentials selects a credential provider by type. Mutually exclusive
	// with Forward. The Type must be a key in the providers map passed to
	// [LoadMuxFromConfig].
	Credentials *CredentialsConfig `yaml:"credentials,omitempty"`
}

// MatchConfig mirrors the [Route] fields in a YAML-friendly shape so the
// match criteria can be expressed as a nested YAML object.
type MatchConfig struct {
	VendorID      string            `yaml:"vendor_id,omitempty"`
	MarketplaceID string            `yaml:"marketplace_id,omitempty"`
	ProductID     string            `yaml:"product_id,omitempty"`
	EnvironmentID string            `yaml:"environment_id,omitempty"`
	TargetURL     string            `yaml:"target_url,omitempty"`
	Data          map[string]string `yaml:"data,omitempty"`
}

// CredentialsConfig identifies which pre-built credential provider should
// handle a route. Only Type is interpreted by [LoadMuxFromConfig]: it's a
// discriminator used to look up an [sdk.CredentialProvider] in the providers
// map.
//
// Provider-specific configuration (OAuth endpoints, scopes, etc.) is the
// caller's responsibility — they construct the providers and register them
// in the lookup map before calling [LoadMuxFromConfig]. This keeps the mux
// loader decoupled from provider internals, which vary widely.
type CredentialsConfig struct {
	// Type is the discriminator used to look up the provider.
	Type string `yaml:"type"`
}

// MuxFallbackConfig is the catch-all entry used when no route matches.
//
// Only Credentials is supported in v1. Setting Forward returns a
// configuration error: a silent fallback-forward would route any
// unmatched request — including misconfigured or unexpected traffic — to
// an upstream without credential injection, which is ambiguous and unsafe.
// Forward routes must be explicit per-match.
type MuxFallbackConfig struct {
	Credentials *CredentialsConfig `yaml:"credentials,omitempty"`
	// Forward is rejected by [LoadMuxFromConfig]. Documented as a field
	// so a misconfigured YAML can be diagnosed with a clear error rather
	// than silently ignored.
	Forward string `yaml:"forward,omitempty"`
}

// LoadMuxFromConfig builds a [*Mux] from a [MuxConfig] and a lookup of
// pre-built credential providers keyed by [CredentialsConfig.Type].
//
// Validation rules:
//   - Every route must set exactly one of forward or credentials.
//   - A route's credentials.type must be non-empty and present in providers.
//   - The fallback, if present, must set credentials (not forward) and the
//     credentials.type must be non-empty and present in providers.
//
// On the first validation failure, an error is returned that names the
// offending route by index (e.g. "routes[2]") or "fallback". No partial
// mux is returned on error.
func LoadMuxFromConfig(cfg MuxConfig, providers map[string]sdk.CredentialProvider) (*Mux, error) {
	m := NewMux()

	for i, rc := range cfg.Routes {
		if err := applyRoute(m, i, rc, providers); err != nil {
			return nil, err
		}
	}

	if cfg.Fallback != nil {
		if err := applyFallback(m, cfg.Fallback, providers); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// applyRoute validates and registers a single route entry.
func applyRoute(m *Mux, i int, rc MuxRouteConfig, providers map[string]sdk.CredentialProvider) error {
	hasForward := rc.Forward != ""
	hasCreds := rc.Credentials != nil
	if hasForward == hasCreds {
		return fmt.Errorf("routes[%d]: exactly one of forward or credentials must be set", i)
	}

	route := routeFromMatch(rc.Match)

	if hasForward {
		m.HandleForward(route, rc.Forward)
		return nil
	}

	if rc.Credentials.Type == "" {
		return fmt.Errorf("routes[%d]: credentials.type must be non-empty", i)
	}
	p, ok := providers[rc.Credentials.Type]
	if !ok {
		return fmt.Errorf("routes[%d]: unknown credentials provider type %q", i, rc.Credentials.Type)
	}
	m.Handle(route, p)
	return nil
}

// applyFallback validates and installs the catch-all provider.
// Forward is rejected — see [MuxFallbackConfig] for rationale.
func applyFallback(m *Mux, fc *MuxFallbackConfig, providers map[string]sdk.CredentialProvider) error {
	if fc.Forward != "" {
		return fmt.Errorf("fallback: forward is not supported; fallback must use credentials")
	}
	if fc.Credentials == nil {
		return fmt.Errorf("fallback: credentials must be set")
	}
	if fc.Credentials.Type == "" {
		return fmt.Errorf("fallback: credentials.type must be non-empty")
	}
	p, ok := providers[fc.Credentials.Type]
	if !ok {
		return fmt.Errorf("fallback: unknown credentials provider type %q", fc.Credentials.Type)
	}
	m.Default(p)
	return nil
}

// routeFromMatch projects a MatchConfig into a Route.
func routeFromMatch(mc MatchConfig) Route {
	return Route{
		VendorID:      mc.VendorID,
		MarketplaceID: mc.MarketplaceID,
		ProductID:     mc.ProductID,
		EnvironmentID: mc.EnvironmentID,
		TargetURL:     mc.TargetURL,
		Data:          mc.Data,
	}
}
