// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Sentinel errors for allow-list validation.
var (
	// ErrEmptyAllowList is returned when the allow list is empty or nil.
	ErrEmptyAllowList = errors.New("allow list is empty")

	// ErrHostNotAllowed is returned when the target host is not in the allow list.
	ErrHostNotAllowed = errors.New("host not allowed")

	// ErrPathNotAllowed is returned when the target path is not in the allow list for the host.
	ErrPathNotAllowed = errors.New("path not allowed")

	// ErrInvalidTargetURL is returned when the target URL cannot be parsed.
	ErrInvalidTargetURL = errors.New("invalid target URL")

	// ErrPathTraversalDetected is returned when path traversal is detected.
	ErrPathTraversalDetected = errors.New("path traversal detected")
)

// AllowListValidator validates target URLs against an allow list.
// It implements "Default Deny" - all requests are blocked unless
// explicitly allowed by the configured patterns.
type AllowListValidator struct {
	// hostPatterns maps lowercased domain patterns to their allowed path patterns.
	hostPatterns map[string][]string
	// isEmpty indicates whether the allow list is empty (denies all).
	isEmpty bool
}

// NewAllowListValidator creates a new validator from an allow list configuration.
// If allowList is nil or empty, all requests will be denied.
func NewAllowListValidator(allowList map[string][]string) *AllowListValidator {
	v := &AllowListValidator{
		hostPatterns: make(map[string][]string),
		isEmpty:      len(allowList) == 0,
	}

	// Normalize host patterns to lowercase
	for host, paths := range allowList {
		v.hostPatterns[strings.ToLower(host)] = paths
	}

	return v
}

// Validate checks if the target URL is allowed by the allow list.
// It returns nil if the URL is allowed, or an error describing why it was blocked.
//
// The validation process:
//  1. Parse the URL
//  2. Validate scheme and normalize host (lowercase)
//  3. Resolve effective target port (explicit or scheme default)
//  4. Check host+port matches any domain pattern in allow list
//  5. Check if path matches any path pattern for the matched domain
//
// Security checks:
//   - Path traversal detection (../ and encoded variants)
//   - Query parameters are ignored during path matching
func (v *AllowListValidator) Validate(targetURL string) error {
	if v.isEmpty {
		return ErrEmptyAllowList
	}

	if targetURL == "" {
		return fmt.Errorf("%w: empty URL", ErrInvalidTargetURL)
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("parsing URL: %w", ErrInvalidTargetURL)
	}

	// Validate URL has required components
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: missing scheme or host", ErrInvalidTargetURL)
	}

	// Extract and normalize host (lowercase, strip port)
	host := strings.ToLower(parsed.Hostname())

	targetPort, err := resolveTargetPort(parsed)
	if err != nil {
		return err
	}

	// Get path, defaulting to "/" if empty
	path := parsed.Path
	if path == "" {
		path = "/"
	}

	// Security: Check for path traversal attacks
	if err := checkPathTraversal(path); err != nil {
		return err
	}

	// Find matching host pattern
	allowedPaths, ok := v.findMatchingHost(host, targetPort, parsed.Scheme)
	if !ok {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
	}

	// Check if path matches any allowed pattern
	if !v.pathMatches(path, allowedPaths) {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, path)
	}

	return nil
}

// findMatchingHost finds the allow list entry that matches the given host and port.
// It tries exact match first, then glob patterns, while enforcing port constraints.
// Returns the allowed paths and true if a match is found.
func (v *AllowListValidator) findMatchingHost(host string, targetPort int, scheme string) ([]string, bool) {
	// Try exact match first
	if paths, ok := v.hostPatterns[host]; ok {
		if portMatches(targetPort, scheme, false, 0) {
			return paths, true
		}
	}

	// Try exact host with explicit port first
	for pattern, paths := range v.hostPatterns {
		hostPattern, configuredPort, hasPort, err := splitHostPattern(pattern)
		if err != nil {
			continue
		}

		if hostPattern == host && portMatches(targetPort, scheme, hasPort, configuredPort) {
			return paths, true
		}
	}

	// Try glob patterns
	for pattern, paths := range v.hostPatterns {
		hostPattern, configuredPort, hasPort, err := splitHostPattern(pattern)
		if err != nil {
			continue
		}

		if GlobMatch(hostPattern, host, '.') && portMatches(targetPort, scheme, hasPort, configuredPort) {
			return paths, true
		}
	}

	return nil, false
}

func resolveTargetPort(parsed *url.URL) (int, error) {
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return 0, fmt.Errorf("%w: invalid port", ErrInvalidTargetURL)
		}
		return port, nil
	}

	defaultPort, ok := defaultPortForScheme(parsed.Scheme)
	if !ok {
		return 0, fmt.Errorf("%w: unsupported scheme %q", ErrInvalidTargetURL, parsed.Scheme)
	}

	return defaultPort, nil
}

func defaultPortForScheme(scheme string) (int, bool) {
	switch strings.ToLower(scheme) {
	case "https":
		return 443, true
	case "http":
		return 80, true
	default:
		return 0, false
	}
}

func splitHostPattern(pattern string) (hostPattern string, port int, hasPort bool, err error) {
	idx := strings.LastIndex(pattern, ":")
	if idx == -1 {
		return pattern, 0, false, nil
	}

	host := pattern[:idx]
	portPart := pattern[idx+1:]
	if host == "" || portPart == "" {
		return "", 0, false, errors.New("invalid host:port pattern")
	}

	port, err = strconv.Atoi(portPart)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false, errors.New("invalid host port")
	}

	return host, port, true, nil
}

func portMatches(targetPort int, scheme string, hasConfiguredPort bool, configuredPort int) bool {
	if hasConfiguredPort {
		return targetPort == configuredPort
	}

	defaultPort, ok := defaultPortForScheme(scheme)
	if !ok {
		return false
	}

	return targetPort == defaultPort
}

// pathMatches checks if the given path matches any of the allowed patterns.
func (v *AllowListValidator) pathMatches(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if GlobMatch(pattern, path, '/') {
			return true
		}
	}
	return false
}

// checkPathTraversal detects path traversal attempts in URLs.
// It checks for:
//   - Literal ".." sequences
//   - URL-encoded ".." (%2e%2e, %2E%2E)
//   - Double-encoded ".." (%252e%252e)
//   - Backslash variants (..\)
func checkPathTraversal(path string) error {
	// Decode the path to catch encoded traversal attempts
	decoded, err := url.PathUnescape(path)
	if err != nil {
		// If we can't decode, reject it as suspicious
		return fmt.Errorf("%w: unable to decode path", ErrPathTraversalDetected)
	}

	// Double-decode to catch double-encoded attacks
	doubleDecoded, _ := url.PathUnescape(decoded)

	// Check all variants for traversal patterns
	for _, p := range []string{path, decoded, doubleDecoded} {
		if containsTraversal(p) {
			return ErrPathTraversalDetected
		}
	}

	return nil
}

// containsTraversal checks if a path contains traversal sequences.
func containsTraversal(path string) bool {
	// Normalize backslashes to forward slashes for consistent checking
	normalized := strings.ReplaceAll(path, "\\", "/")

	// Check for ".." in path segments
	segments := strings.Split(normalized, "/")
	for _, seg := range segments {
		if seg == ".." {
			return true
		}
	}

	return false
}

// ValidateAllowListConfig validates the glob patterns in an allow list configuration.
// This should be called at config load time to fail fast on invalid patterns.
func ValidateAllowListConfig(allowList map[string][]string) error {
	if allowList == nil {
		return nil
	}

	var errs []error

	for host, paths := range allowList {
		hostPattern, _, _, err := splitHostPattern(host)
		if err != nil {
			errs = append(errs, fmt.Errorf("host %q: invalid port", host))
			continue
		}

		// Validate host pattern
		if err := ValidateGlobPattern(hostPattern, '.'); err != nil {
			errs = append(errs, fmt.Errorf("host %q: %w", host, err))
		}

		// Validate path patterns
		for _, path := range paths {
			if err := ValidateGlobPattern(path, '/'); err != nil {
				errs = append(errs, fmt.Errorf("host %q path %q: %w", host, path, err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
