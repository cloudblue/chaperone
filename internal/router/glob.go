// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package router provides URL validation and routing logic for the
// Chaperone egress proxy. It implements allow-list based traffic control
// with glob pattern matching for hosts and paths.
package router

import (
	"errors"
	"fmt"
	"strings"
)

// GlobMatch tests whether the input matches the glob pattern using the
// specified separator. It supports two types of wildcards:
//   - * (single-level): matches any characters within one segment (does NOT cross separators)
//   - ** (recursive): matches any characters including separators (crosses multiple segments)
//
// Examples with separator '.':
//
//	GlobMatch("*.google.com", "api.google.com", '.')     -> true
//	GlobMatch("*.google.com", "a.b.google.com", '.')     -> false
//	GlobMatch("**.google.com", "a.b.google.com", '.')    -> true
//
// Examples with separator '/':
//
//	GlobMatch("/v1/*", "/v1/customers", '/')             -> true
//	GlobMatch("/v1/*", "/v1/customers/123", '/')         -> false
//	GlobMatch("/v1/**", "/v1/customers/123", '/')        -> true
func GlobMatch(pattern, input string, sep byte) bool {
	return globMatch(pattern, input, sep)
}

// globMatch is the internal recursive implementation of glob matching.
// The logic is split into separate functions for each case to keep complexity low.
func globMatch(pattern, input string, sep byte) bool {
	switch {
	case pattern == "":
		return input == ""
	case strings.HasPrefix(pattern, "**"):
		return matchDoubleStar(pattern, input, sep)
	case strings.HasPrefix(pattern, "*"):
		return matchSingleStar(pattern, input, sep)
	default:
		return matchLiteralPrefix(pattern, input, sep)
	}
}

// matchDoubleStar handles ** (recursive wildcard) matching.
func matchDoubleStar(pattern, input string, sep byte) bool {
	rest := strings.TrimPrefix(pattern, "**")

	// ** at end matches everything (including empty)
	if rest == "" {
		return true
	}

	// Skip separator after ** if present
	if rest != "" && rest[0] == sep {
		rest = rest[1:]
	}

	// ** can match zero or more segments
	// If rest is empty after skipping separator, we've matched
	if rest == "" {
		return true
	}

	// Try matching rest of pattern at every position in input
	// (including matching zero segments)
	for i := 0; i <= len(input); i++ {
		// Only try at start or after a separator
		if i == 0 || (i > 0 && input[i-1] == sep) {
			if globMatch(rest, input[i:], sep) {
				return true
			}
		}
	}
	return false
}

// matchSingleStar handles * (single-level wildcard) matching.
func matchSingleStar(pattern, input string, sep byte) bool {
	rest := strings.TrimPrefix(pattern, "*")

	// * at end matches one segment (no separator in remaining input)
	if rest == "" {
		// Must match at least one character and no separator
		if input == "" {
			return false
		}
		return !strings.ContainsRune(input, rune(sep))
	}

	// Skip separator after * if present
	if rest != "" && rest[0] == sep {
		// Find end of current segment in input
		endIdx := strings.IndexByte(input, sep)
		if endIdx == -1 {
			// No separator in input - * must match entire input
			return false
		}
		// * must match at least one character
		if endIdx == 0 {
			return false
		}
		// Continue matching after the separator
		return globMatch(rest[1:], input[endIdx+1:], sep)
	}

	// * followed by non-separator char: try matching at each position
	// within the current segment
	endIdx := strings.IndexByte(input, sep)
	if endIdx == -1 {
		endIdx = len(input)
	}

	// Try matching rest of pattern at every position within segment
	for i := 1; i <= endIdx; i++ {
		if globMatch(rest, input[i:], sep) {
			return true
		}
	}
	return false
}

// matchLiteralPrefix handles matching when pattern starts with literal characters.
func matchLiteralPrefix(pattern, input string, sep byte) bool {
	// No wildcard at start - match character by character
	if input == "" {
		// Pattern not empty but input is empty
		// Special case: remaining pattern is /**  (separator + double star)
		return pattern == string(sep)+"**"
	}

	// Find next wildcard position in pattern
	nextStar := strings.IndexByte(pattern, '*')
	if nextStar == -1 {
		// No more wildcards - exact match required
		return pattern == input
	}

	// Match literal prefix before next wildcard
	prefix := pattern[:nextStar]

	// Special handling for /** pattern (separator before **)
	// This should match even if input doesn't have the trailing separator
	if len(pattern) > nextStar+1 && pattern[nextStar:nextStar+2] == "**" {
		return matchPrefixBeforeDoubleStar(pattern, input, prefix, nextStar, sep)
	}

	if !strings.HasPrefix(input, prefix) {
		return false
	}

	// Continue matching after literal prefix
	return globMatch(pattern[nextStar:], input[len(prefix):], sep)
}

// matchPrefixBeforeDoubleStar handles the special case of literal prefix followed by **.
func matchPrefixBeforeDoubleStar(pattern, input, prefix string, nextStar int, sep byte) bool {
	// Check if prefix ends with separator
	if prefix != "" && prefix[len(prefix)-1] == sep {
		// Try matching with prefix including separator
		if strings.HasPrefix(input, prefix) {
			return globMatch(pattern[nextStar:], input[len(prefix):], sep)
		}
		// Also try matching with prefix excluding the trailing separator
		// This allows /v1/** to match /v1
		prefixNoSep := prefix[:len(prefix)-1]
		if input == prefixNoSep {
			// Input matches prefix without separator, ** matches empty
			return true
		}
		if strings.HasPrefix(input, prefixNoSep+string(sep)) {
			return globMatch(pattern[nextStar:], input[len(prefixNoSep):], sep)
		}
		return false
	}

	// Prefix doesn't end with separator - standard handling
	if !strings.HasPrefix(input, prefix) {
		return false
	}
	return globMatch(pattern[nextStar:], input[len(prefix):], sep)
}

// ValidateGlobPattern checks if a glob pattern has valid syntax.
// Valid patterns:
//   - Exact strings: "api.google.com", "/v1/customers"
//   - Single wildcard segment: "*.google.com", "/v1/*/profiles"
//   - Recursive wildcard: "**.google.com", "/v1/**"
//
// Invalid patterns:
//   - Partial wildcards in segment: "api*.google.com", "/v1/cust*/profiles"
//   - Triple or more stars: "***.google.com"
func ValidateGlobPattern(pattern string, sep byte) error {
	if pattern == "" {
		return nil
	}

	sepStr := string(sep)
	segments := strings.Split(pattern, sepStr)

	for _, seg := range segments {
		if err := validateSegment(seg); err != nil {
			return fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
	}

	return nil
}

// ErrInvalidGlobPattern is returned when a glob pattern has invalid syntax.
var ErrInvalidGlobPattern = errors.New("invalid glob pattern")

// validateSegment checks if a single segment of a glob pattern is valid.
// Valid segments:
//   - Empty string (from leading/trailing separators)
//   - Literal strings with no wildcards
//   - Single "*"
//   - Double "**"
//
// Invalid segments:
//   - Partial wildcards: "abc*", "*abc", "a*b"
//   - Triple or more stars: "***"
func validateSegment(seg string) error {
	if seg == "" || seg == "*" || seg == "**" {
		return nil
	}

	// Check for any wildcards in the segment
	if strings.Contains(seg, "*") {
		return fmt.Errorf("%w: wildcard must be alone in segment, got %q", ErrInvalidGlobPattern, seg)
	}

	return nil
}
