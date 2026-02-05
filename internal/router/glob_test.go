// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import "testing"

func TestGlobMatch_SingleStar_Domain(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "single star matches one segment",
			pattern:   "*.google.com",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "single star does not match two segments",
			pattern:   "*.google.com",
			input:     "a.b.google.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "exact match no wildcards",
			pattern:   "api.google.com",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "exact match fails different input",
			pattern:   "api.google.com",
			input:     "other.google.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "single star at end",
			pattern:   "api.google.*",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "single star in middle",
			pattern:   "api.*.com",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "single star in middle fails two segments",
			pattern:   "api.*.com",
			input:     "api.a.b.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "multiple single stars",
			pattern:   "*.*.com",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "star matches empty segment - should not match",
			pattern:   "*.google.com",
			input:     ".google.com",
			sep:       '.',
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

func TestGlobMatch_DoubleStar_Domain(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "double star matches multiple segments",
			pattern:   "**.amazonaws.com",
			input:     "a.b.c.amazonaws.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star matches single segment",
			pattern:   "**.amazonaws.com",
			input:     "s3.amazonaws.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star matches zero segments",
			pattern:   "**.example.com",
			input:     "example.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star at end",
			pattern:   "api.**",
			input:     "api.google.com.au",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star in middle",
			pattern:   "api.**.io",
			input:     "api.a.b.c.io",
			sep:       '.',
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

func TestGlobMatch_SingleStar_Path(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "single star matches one segment",
			pattern:   "/v1/customers/*",
			input:     "/v1/customers/123",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "single star does not match nested path",
			pattern:   "/v1/customers/*",
			input:     "/v1/customers/123/profile",
			sep:       '/',
			wantMatch: false,
		},
		{
			name:      "single star in middle",
			pattern:   "/v1/customers/*/profiles",
			input:     "/v1/customers/123/profiles",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "single star in middle fails nested",
			pattern:   "/v1/customers/*/profiles",
			input:     "/v1/customers/123/456/profiles",
			sep:       '/',
			wantMatch: false,
		},
		{
			name:      "exact path match",
			pattern:   "/v1/customers",
			input:     "/v1/customers",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "exact path fails",
			pattern:   "/v1/customers",
			input:     "/v1/orders",
			sep:       '/',
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

func TestGlobMatch_DoubleStar_Path(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "double star matches nested paths",
			pattern:   "/v1/**",
			input:     "/v1/customers/123/profile",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "double star matches single segment",
			pattern:   "/v1/**",
			input:     "/v1/customers",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "double star matches zero segments",
			pattern:   "/v1/**",
			input:     "/v1",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "double star with trailing content",
			pattern:   "/v1/**/status",
			input:     "/v1/customers/123/status",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "double star with trailing content - deep nesting",
			pattern:   "/v1/**/status",
			input:     "/v1/a/b/c/d/status",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "catch all pattern",
			pattern:   "/**",
			input:     "/anything/deeply/nested",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "catch all matches root",
			pattern:   "/**",
			input:     "/",
			sep:       '/',
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

func TestGlobMatch_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "empty pattern matches empty input",
			pattern:   "",
			input:     "",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "empty pattern does not match non-empty input",
			pattern:   "",
			input:     "something",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "double star only matches anything",
			pattern:   "**",
			input:     "anything.here",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "single star only matches single segment",
			pattern:   "*",
			input:     "anything",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "single star only does not match multiple segments",
			pattern:   "*",
			input:     "a.b",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "trailing separator in pattern",
			pattern:   "/api/",
			input:     "/api/",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "trailing separator in input only",
			pattern:   "/api",
			input:     "/api/",
			sep:       '/',
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

func TestValidateGlobPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		sep     byte
		wantErr bool
	}{
		{
			name:    "valid exact pattern",
			pattern: "api.google.com",
			sep:     '.',
			wantErr: false,
		},
		{
			name:    "valid single star at start",
			pattern: "*.google.com",
			sep:     '.',
			wantErr: false,
		},
		{
			name:    "valid double star at start",
			pattern: "**.google.com",
			sep:     '.',
			wantErr: false,
		},
		{
			name:    "valid single star in middle",
			pattern: "/v1/*/status",
			sep:     '/',
			wantErr: false,
		},
		{
			name:    "valid double star in middle",
			pattern: "/v1/**/status",
			sep:     '/',
			wantErr: false,
		},
		{
			name:    "valid catch-all",
			pattern: "/**",
			sep:     '/',
			wantErr: false,
		},
		{
			name:    "invalid triple star",
			pattern: "***.google.com",
			sep:     '.',
			wantErr: true,
		},
		{
			name:    "invalid star in segment",
			pattern: "api*.google.com",
			sep:     '.',
			wantErr: true,
		},
		{
			name:    "invalid partial star in segment",
			pattern: "/v1/cust*/profiles",
			sep:     '/',
			wantErr: true,
		},
		{
			name:    "invalid star followed by chars",
			pattern: "/v1/*abc/profiles",
			sep:     '/',
			wantErr: true,
		},
		{
			name:    "empty pattern is valid",
			pattern: "",
			sep:     '.',
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGlobPattern(tt.pattern, tt.sep)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGlobPattern(%q, %q) error = %v, wantErr %v",
					tt.pattern, tt.sep, err, tt.wantErr)
			}
		})
	}
}

// TestGlobMatch_SingleStar_MoreCases tests additional edge cases for single star matching
// to improve coverage of matchSingleStar function.
func TestGlobMatch_SingleStar_MoreCases(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		// * followed by non-separator char within segment
		{
			name:      "star followed by literal within segment - match",
			pattern:   "*xy.com",
			input:     "abcxy.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "star followed by literal within segment - no match",
			pattern:   "*xy.com",
			input:     "abc.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "star at end with empty remaining",
			pattern:   "api.*",
			input:     "api.",
			sep:       '.',
			wantMatch: false, // * requires at least one char
		},
		{
			name:      "star matches entire input",
			pattern:   "*",
			input:     "anything",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "star does not match with separator",
			pattern:   "*",
			input:     "a.b",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "star with no separator in rest of input",
			pattern:   "*.com",
			input:     "api.com",
			sep:       '.',
			wantMatch: true,
		},
		// Edge case: * followed by literal, input has separator before literal match
		{
			name:      "star before separator literal mismatch",
			pattern:   "*z.com",
			input:     "a.b.com",
			sep:       '.',
			wantMatch: false,
		},
		// Path separator tests for single star
		{
			name:      "path single star matches segment",
			pattern:   "/api/*/info",
			input:     "/api/users/info",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "path single star at end",
			pattern:   "/api/*",
			input:     "/api/users",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "path single star empty match fails",
			pattern:   "/api/*/info",
			input:     "/api//info",
			sep:       '/',
			wantMatch: false, // * requires at least one char
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

// TestGlobMatch_DoubleStar_MoreCases tests additional edge cases for double star matching.
func TestGlobMatch_DoubleStar_MoreCases(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "double star with content before and after",
			pattern:   "api.**z.com",
			input:     "api.x.y.z.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star matches empty",
			pattern:   "**",
			input:     "",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star only matches single segment",
			pattern:   "**",
			input:     "api",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star in middle matches zero segments",
			pattern:   "api.**.com",
			input:     "api.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star between separators",
			pattern:   "/v1/**/end",
			input:     "/v1/end",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "double star path with trailing content - no match",
			pattern:   "/v1/**/status",
			input:     "/v1/customers/profile",
			sep:       '/',
			wantMatch: false,
		},
		// Test edge: ** at very start
		{
			name:      "double star at start matches anything before suffix",
			pattern:   "**.io",
			input:     "a.b.c.io",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "double star at start matches just suffix",
			pattern:   "**.io",
			input:     "io",
			sep:       '.',
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

// TestGlobMatch_LiteralPrefix tests the matchLiteralPrefix function edge cases.
func TestGlobMatch_LiteralPrefix(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		// Note: Partial wildcards like "api*.com" are INVALID patterns per validation.
		// GlobMatch will still process them (matches from validation pass to runtime),
		// but users should never have them due to config validation.
		// We test valid patterns that exercise matchLiteralPrefix.
		{
			name:      "exact match",
			pattern:   "api.google.com",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "prefix mismatch",
			pattern:   "api.google.com",
			input:     "other.google.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "pattern longer than input",
			pattern:   "api.google.com.au",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "input longer than pattern",
			pattern:   "api.google.com",
			input:     "api.google.com.au",
			sep:       '.',
			wantMatch: false,
		},
		// Test matchPrefixBeforeDoubleStar edge cases
		{
			name:      "prefix ending with separator before **",
			pattern:   "/v1/**",
			input:     "/v1",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "prefix ending with separator before ** - with content",
			pattern:   "/v1/**",
			input:     "/v1/customers/123",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "prefix not ending with separator before **",
			pattern:   "/v1**",
			input:     "/v1/extra",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "prefix before ** - input shorter than prefix without sep",
			pattern:   "/v1/api/**",
			input:     "/v1",
			sep:       '/',
			wantMatch: false,
		},
		{
			name:      "prefix before ** - partial prefix match fails",
			pattern:   "/v1/api/**",
			input:     "/v1/other/path",
			sep:       '/',
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

// TestGlobMatch_NegativeCases tests patterns that should NOT match for security.
func TestGlobMatch_NegativeCases(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "single star should not cross separators",
			pattern:   "*.com",
			input:     "a.b.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "pattern should not match substring",
			pattern:   "api.com",
			input:     "evilapi.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "pattern should not match superstring",
			pattern:   "api.google.com",
			input:     "api.google",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "empty input with non-empty pattern",
			pattern:   "api.com",
			input:     "",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "path pattern exact should not match with extra",
			pattern:   "/v1/customers",
			input:     "/v1/customers/extra",
			sep:       '/',
			wantMatch: false,
		},
		{
			name:      "path single star should not match nested",
			pattern:   "/v1/*/profile",
			input:     "/v1/a/b/profile",
			sep:       '/',
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, tt.sep)
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.input, tt.sep, got, tt.wantMatch)
			}
		})
	}
}

// FuzzGlobMatch tests glob matching with random inputs to find edge cases.
func FuzzGlobMatch(f *testing.F) {
	// Add seed corpus
	seeds := []struct {
		pattern string
		input   string
	}{
		{"*.google.com", "api.google.com"},
		{"**.amazonaws.com", "s3.us-east-1.amazonaws.com"},
		{"/v1/**", "/v1/customers/123"},
		{"/v1/*/profile", "/v1/user/profile"},
		{"api.com", "api.com"},
		{"", ""},
		{"*", "anything"},
		{"**", "a.b.c"},
		{"a.*.c", "a.b.c"},
		{"a.**.c", "a.x.y.z.c"},
	}

	for _, seed := range seeds {
		f.Add(seed.pattern, seed.input, byte('.'))
		f.Add(seed.pattern, seed.input, byte('/'))
	}

	f.Fuzz(func(t *testing.T, pattern, input string, sep byte) {
		// Skip invalid separators
		if sep == 0 || sep == '*' {
			return
		}

		// The function should not panic
		_ = GlobMatch(pattern, input, sep)
	})
}

// FuzzValidateGlobPattern tests pattern validation with random inputs.
func FuzzValidateGlobPattern(f *testing.F) {
	// Add seed corpus
	seeds := []string{
		"*.google.com",
		"**.amazonaws.com",
		"/v1/**",
		"/v1/*/profile",
		"api.com",
		"",
		"*",
		"**",
		"***",
		"api*.com",
		"*api.com",
	}

	for _, seed := range seeds {
		f.Add(seed, byte('.'))
		f.Add(seed, byte('/'))
	}

	f.Fuzz(func(t *testing.T, pattern string, sep byte) {
		// Skip invalid separators
		if sep == 0 || sep == '*' {
			return
		}

		// The function should not panic
		err := ValidateGlobPattern(pattern, sep)

		// If validation passes, matching should not panic
		if err == nil {
			_ = GlobMatch(pattern, "test.input.com", sep)
			_ = GlobMatch(pattern, "/test/input/path", sep)
		}
	})
}
