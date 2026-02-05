// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"errors"
	"fmt"
	"net/url"
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
//  2. Extract and normalize the host (lowercase, strip port)
//  3. Check if host matches any domain pattern in allow list
//  4. Check if path matches any path pattern for the matched domain
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
	allowedPaths, ok := v.findMatchingHost(host)
	if !ok {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
	}

	// Check if path matches any allowed pattern
	if !v.pathMatches(path, allowedPaths) {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, path)
	}

	return nil
}

// findMatchingHost finds the allow list entry that matches the given host.
// It tries exact match first, then glob patterns.
// Returns the allowed paths and true if a match is found.
func (v *AllowListValidator) findMatchingHost(host string) ([]string, bool) {
	// Try exact match first
	if paths, ok := v.hostPatterns[host]; ok {
		return paths, true
	}

	// Try glob patterns
	for pattern, paths := range v.hostPatterns {
		if GlobMatch(pattern, host, '.') {
			return paths, true
		}
	}

	return nil, false
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
		// Validate host pattern
		if err := ValidateGlobPattern(host, '.'); err != nil {
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
