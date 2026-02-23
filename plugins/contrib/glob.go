// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import "strings"

// GlobMatch tests whether the input matches the glob pattern using the
// specified separator. It supports two types of wildcards:
//   - * (single-level): matches any characters within one segment (does NOT cross separators)
//   - ** (recursive): matches any characters including separators (crosses multiple segments)
//
// This is an independent copy of the glob matcher from the core module.
// The two matchers serve different contexts (allow-list routing vs mux routing)
// and may diverge over time.
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
	if rest[0] == sep {
		rest = rest[1:]
	}

	// ** can match zero or more segments
	if rest == "" {
		return true
	}

	// Try matching rest of pattern at every position in input
	for i := 0; i <= len(input); i++ {
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
		if input == "" {
			return false
		}
		return !strings.ContainsRune(input, rune(sep))
	}

	// Skip separator after * if present
	if rest[0] == sep {
		endIdx := strings.IndexByte(input, sep)
		if endIdx == -1 {
			return false
		}
		if endIdx == 0 {
			return false
		}
		return globMatch(rest[1:], input[endIdx+1:], sep)
	}

	// * followed by non-separator char: try matching at each position
	// within the current segment
	endIdx := strings.IndexByte(input, sep)
	if endIdx == -1 {
		endIdx = len(input)
	}

	for i := 1; i <= endIdx; i++ {
		if globMatch(rest, input[i:], sep) {
			return true
		}
	}
	return false
}

// matchLiteralPrefix handles matching when pattern starts with literal characters.
func matchLiteralPrefix(pattern, input string, sep byte) bool {
	if input == "" {
		return pattern == string(sep)+"**"
	}

	nextStar := strings.IndexByte(pattern, '*')
	if nextStar == -1 {
		return pattern == input
	}

	prefix := pattern[:nextStar]

	// Special handling for /** pattern (separator before **)
	if len(pattern) > nextStar+1 && pattern[nextStar:nextStar+2] == "**" {
		return matchPrefixBeforeDoubleStar(pattern, input, prefix, nextStar, sep)
	}

	if !strings.HasPrefix(input, prefix) {
		return false
	}

	return globMatch(pattern[nextStar:], input[len(prefix):], sep)
}

// matchPrefixBeforeDoubleStar handles the special case of literal prefix followed by **.
func matchPrefixBeforeDoubleStar(pattern, input, prefix string, nextStar int, sep byte) bool {
	if prefix != "" && prefix[len(prefix)-1] == sep {
		if strings.HasPrefix(input, prefix) {
			return globMatch(pattern[nextStar:], input[len(prefix):], sep)
		}
		prefixNoSep := prefix[:len(prefix)-1]
		if input == prefixNoSep {
			return true
		}
		if strings.HasPrefix(input, prefixNoSep+string(sep)) {
			return globMatch(pattern[nextStar:], input[len(prefixNoSep):], sep)
		}
		return false
	}

	if !strings.HasPrefix(input, prefix) {
		return false
	}
	return globMatch(pattern[nextStar:], input[len(prefix):], sep)
}
