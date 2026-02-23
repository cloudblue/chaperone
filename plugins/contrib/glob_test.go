// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import "testing"

func TestGlobMatch_VendorID_ExactMatch(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		wantMatch bool
	}{
		{
			name:      "exact match",
			pattern:   "acme-corp",
			input:     "acme-corp",
			wantMatch: true,
		},
		{
			name:      "different vendor",
			pattern:   "acme-corp",
			input:     "other-vendor",
			wantMatch: false,
		},
		{
			name:      "prefix mismatch",
			pattern:   "acme-corp",
			input:     "acme-corp-extra",
			wantMatch: false,
		},
		{
			name:      "suffix mismatch",
			pattern:   "acme-corp",
			input:     "my-acme-corp",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, '/')
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, '/') = %v, want %v",
					tt.pattern, tt.input, got, tt.wantMatch)
			}
		})
	}
}

func TestGlobMatch_VendorID_SingleStar(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		wantMatch bool
	}{
		{
			name:      "star suffix matches",
			pattern:   "microsoft-*",
			input:     "microsoft-azure",
			wantMatch: true,
		},
		{
			name:      "star suffix matches different value",
			pattern:   "microsoft-*",
			input:     "microsoft-365",
			wantMatch: true,
		},
		{
			name:      "star suffix does not match prefix only",
			pattern:   "microsoft-*",
			input:     "microsoft-",
			wantMatch: false,
		},
		{
			name:      "star suffix does not match different prefix",
			pattern:   "microsoft-*",
			input:     "google-cloud",
			wantMatch: false,
		},
		{
			name:      "star alone matches any vendor",
			pattern:   "*",
			input:     "any-vendor",
			wantMatch: true,
		},
		{
			name:      "star alone does not match empty",
			pattern:   "*",
			input:     "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, '/')
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, '/') = %v, want %v",
					tt.pattern, tt.input, got, tt.wantMatch)
			}
		})
	}
}

func TestGlobMatch_URL_SingleStar(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "star matches one path segment",
			pattern:   "/v1/customers/*",
			input:     "/v1/customers/123",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "star does not match nested path",
			pattern:   "/v1/customers/*",
			input:     "/v1/customers/123/profile",
			sep:       '/',
			wantMatch: false,
		},
		{
			name:      "star in middle matches one segment",
			pattern:   "/v1/customers/*/profiles",
			input:     "/v1/customers/123/profiles",
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

func TestGlobMatch_URL_DoubleStar(t *testing.T) {
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
			name:      "catch all",
			pattern:   "/**",
			input:     "/anything/deeply/nested",
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

func TestGlobMatch_Domain(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		input     string
		sep       byte
		wantMatch bool
	}{
		{
			name:      "single star matches one subdomain",
			pattern:   "*.google.com",
			input:     "api.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "single star does not cross segments",
			pattern:   "*.google.com",
			input:     "a.b.google.com",
			sep:       '.',
			wantMatch: false,
		},
		{
			name:      "double star matches multiple subdomains",
			pattern:   "**.google.com",
			input:     "a.b.google.com",
			sep:       '.',
			wantMatch: true,
		},
		{
			name:      "exact domain match",
			pattern:   "api.google.com",
			input:     "api.google.com",
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

func TestGlobMatch_MuxRouting_TargetURL(t *testing.T) {
	// These tests simulate how the mux routes requests by TargetURL.
	// The scheme is stripped before matching; separator is '/'.
	tests := []struct {
		name      string
		pattern   string
		input     string
		wantMatch bool
	}{
		{
			name:      "host wildcard with path wildcard",
			pattern:   "*.graph.microsoft.com/**",
			input:     "api.graph.microsoft.com/v1/users",
			wantMatch: true,
		},
		{
			name:      "host wildcard with different path",
			pattern:   "*.graph.microsoft.com/**",
			input:     "api.graph.microsoft.com/beta/groups",
			wantMatch: true,
		},
		{
			name:      "host mismatch",
			pattern:   "*.graph.microsoft.com/**",
			input:     "api.other.com/v1/users",
			wantMatch: false,
		},
		{
			name:      "exact host and path",
			pattern:   "api.vendor.com/v1/status",
			input:     "api.vendor.com/v1/status",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.input, '/')
			if got != tt.wantMatch {
				t.Errorf("GlobMatch(%q, %q, '/') = %v, want %v",
					tt.pattern, tt.input, got, tt.wantMatch)
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
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "empty pattern does not match non-empty",
			pattern:   "",
			input:     "something",
			sep:       '/',
			wantMatch: false,
		},
		{
			name:      "double star matches anything",
			pattern:   "**",
			input:     "any/thing/here",
			sep:       '/',
			wantMatch: true,
		},
		{
			name:      "double star matches empty",
			pattern:   "**",
			input:     "",
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
