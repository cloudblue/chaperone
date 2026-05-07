// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"fmt"
	"net/url"
)

// TargetAddrMode controls how much detail of the upstream target URL is
// emitted in the `target_addr` log field.
//
// The mode is configured via observability.log_target_addr (or the
// CHAPERONE_OBSERVABILITY_LOG_TARGET_ADDR env var) and applied uniformly
// across every log line that reports the target.
type TargetAddrMode string

const (
	// TargetAddrModeHost emits only the authority (host[:port]) of the target.
	// No scheme, no path, no query. Default mode — minimum information,
	// maximum safety. Example output: "api.vendor.com:8443".
	TargetAddrModeHost TargetAddrMode = "host"

	// TargetAddrModePath emits scheme://host[:port]/path with the query
	// string stripped. Useful for "what endpoint was called" auditing
	// without leaking sensitive query parameters.
	// Example output: "https://api.vendor.com:8443/v1/users".
	TargetAddrModePath TargetAddrMode = "path"

	// TargetAddrModeFull emits the full URL including the query string
	// but with userinfo (user:pass@) stripped. Use only when explicitly
	// required for audit/debugging — query strings may contain secrets.
	// Example output: "https://api.vendor.com:8443/v1/users?key=val".
	TargetAddrModeFull TargetAddrMode = "full"
)

// ValidTargetAddrModes is the list of accepted target_addr mode values.
var ValidTargetAddrModes = []string{
	string(TargetAddrModeHost),
	string(TargetAddrModePath),
	string(TargetAddrModeFull),
}

// ParseTargetAddrMode returns the mode for the given string, or an error
// if the value is not one of host/path/full.
func ParseTargetAddrMode(s string) (TargetAddrMode, error) {
	switch TargetAddrMode(s) {
	case TargetAddrModeHost, TargetAddrModePath, TargetAddrModeFull:
		return TargetAddrMode(s), nil
	default:
		return "", fmt.Errorf("invalid target_addr mode %q (valid: %v)", s, ValidTargetAddrModes)
	}
}

// FormatTargetAddr formats a raw URL string for logging according to mode.
// Returns "" when rawURL is empty, malformed, or has no host. Userinfo
// is always stripped, in every mode.
//
// Unknown modes default to TargetAddrModeHost — the safest fallback. This
// should never happen in practice because the config layer validates the
// mode before reaching the log site.
func FormatTargetAddr(rawURL string, mode TargetAddrMode) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return FormatTargetAddrFromURL(u, mode)
}

// FormatTargetAddrFromURL is the same as FormatTargetAddr but accepts a
// pre-parsed *url.URL. Use this at sites that already parsed the URL
// (e.g. internal/proxy/server.go) to avoid re-parsing on every request.
//
// The input *url.URL is not mutated. Returns "" if u is nil or has no host.
func FormatTargetAddrFromURL(u *url.URL, mode TargetAddrMode) string {
	if u == nil || u.Host == "" {
		return ""
	}

	switch mode {
	case TargetAddrModeHost:
		// Authority only — no allocation needed.
		return u.Host
	case TargetAddrModePath, TargetAddrModeFull:
		// Clone to avoid mutating the caller's URL, then strip components
		// according to mode. Userinfo and fragment are stripped in both modes;
		// the query is stripped only in path mode.
		cloned := *u
		cloned.User = nil
		cloned.Fragment = ""
		cloned.RawFragment = ""
		if mode == TargetAddrModePath {
			cloned.RawQuery = ""
		}
		return cloned.String()
	default:
		// Unknown mode — fall back to host-only as a safe default.
		return u.Host
	}
}
