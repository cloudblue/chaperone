// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"net/url"
	"testing"
)

func TestParseTargetAddrMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    TargetAddrMode
		wantErr bool
	}{
		{"host", "host", TargetAddrModeHost, false},
		{"path", "path", TargetAddrModePath, false},
		{"full", "full", TargetAddrModeFull, false},
		{"empty", "", "", true},
		{"unknown", "verbose", "", true},
		{"case-sensitive (uppercase rejected)", "HOST", "", true},
		{"trailing space rejected", "host ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTargetAddrMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTargetAddrMode(%q) want error, got %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseTargetAddrMode(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseTargetAddrMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatTargetAddr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		mode  TargetAddrMode
		want  string
	}{
		// host mode — only authority, no scheme.
		{
			name:  "host: bare host",
			input: "https://api.vendor.com/v1/users",
			mode:  TargetAddrModeHost,
			want:  "api.vendor.com",
		},
		{
			name:  "host: preserves port",
			input: "https://api.vendor.com:8443/v1",
			mode:  TargetAddrModeHost,
			want:  "api.vendor.com:8443",
		},
		{
			name:  "host: strips userinfo",
			input: "https://user:pass@api.vendor.com/v1",
			mode:  TargetAddrModeHost,
			want:  "api.vendor.com",
		},
		{
			name:  "host: strips query",
			input: "https://api.vendor.com/v1?token=abc",
			mode:  TargetAddrModeHost,
			want:  "api.vendor.com",
		},
		{
			name:  "host: no scheme in output",
			input: "https://api.vendor.com:8443/v1",
			mode:  TargetAddrModeHost,
			want:  "api.vendor.com:8443", // distinguishes from "https://..."
		},

		// path mode — scheme + host + path, no query, no userinfo.
		{
			name:  "path: full URL",
			input: "https://api.vendor.com/v1/users",
			mode:  TargetAddrModePath,
			want:  "https://api.vendor.com/v1/users",
		},
		{
			name:  "path: strips query",
			input: "https://api.vendor.com/v1/users?api_key=secret&token=abc",
			mode:  TargetAddrModePath,
			want:  "https://api.vendor.com/v1/users",
		},
		{
			name:  "path: strips userinfo",
			input: "https://user:pass@api.vendor.com/v1",
			mode:  TargetAddrModePath,
			want:  "https://api.vendor.com/v1",
		},
		{
			name:  "path: strips fragment",
			input: "https://api.vendor.com/v1#section",
			mode:  TargetAddrModePath,
			want:  "https://api.vendor.com/v1",
		},
		{
			name:  "path: no path keeps host root",
			input: "https://api.vendor.com",
			mode:  TargetAddrModePath,
			want:  "https://api.vendor.com",
		},
		{
			name:  "path: preserves port",
			input: "https://api.vendor.com:8443/v1/users",
			mode:  TargetAddrModePath,
			want:  "https://api.vendor.com:8443/v1/users",
		},
		{
			name:  "path: keeps sensitive path segments (operator opted in)",
			input: "https://api.vendor.com/users/alice@example.com",
			mode:  TargetAddrModePath,
			want:  "https://api.vendor.com/users/alice@example.com",
		},

		// full mode — everything except userinfo and fragment.
		{
			name:  "full: preserves query",
			input: "https://api.vendor.com/v1?key=val&token=abc",
			mode:  TargetAddrModeFull,
			want:  "https://api.vendor.com/v1?key=val&token=abc",
		},
		{
			name:  "full: strips userinfo",
			input: "https://user:pass@api.vendor.com/v1?key=val",
			mode:  TargetAddrModeFull,
			want:  "https://api.vendor.com/v1?key=val",
		},
		{
			name:  "full: strips fragment",
			input: "https://api.vendor.com/v1?key=val#section",
			mode:  TargetAddrModeFull,
			want:  "https://api.vendor.com/v1?key=val",
		},
		{
			name:  "full: combined — strips userinfo+fragment, keeps everything else",
			input: "https://u:p@api.vendor.com:8443/v1/users/alice@example.com?token=abc#frag",
			mode:  TargetAddrModeFull,
			want:  "https://api.vendor.com:8443/v1/users/alice@example.com?token=abc",
		},

		// invalid input — return empty string, never panic.
		{"invalid: parse error", "://invalid", TargetAddrModeFull, ""},
		{"invalid: empty", "", TargetAddrModeFull, ""},
		{"invalid: relative URL with no host", "/just/a/path", TargetAddrModeHost, ""},
		{"invalid: path-only no host (path mode)", "/users/123", TargetAddrModePath, ""},

		// unknown mode falls back to host (safe default).
		{
			name:  "unknown mode: falls back to host-only",
			input: "https://api.vendor.com/v1?token=abc",
			mode:  TargetAddrMode("unknown"),
			want:  "api.vendor.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTargetAddr(tt.input, tt.mode)
			if got != tt.want {
				t.Errorf("FormatTargetAddr(%q, %q) = %q, want %q", tt.input, tt.mode, got, tt.want)
			}
		})
	}
}

// TestFormatTargetAddrFromURL_NilSafe verifies the nil-URL guard.
func TestFormatTargetAddrFromURL_NilSafe(t *testing.T) {
	if got := FormatTargetAddrFromURL(nil, TargetAddrModeHost); got != "" {
		t.Errorf("FormatTargetAddrFromURL(nil, host) = %q, want %q", got, "")
	}
	if got := FormatTargetAddrFromURL(&url.URL{}, TargetAddrModeFull); got != "" {
		t.Errorf("FormatTargetAddrFromURL(empty URL, full) = %q, want %q", got, "")
	}
}

// TestFormatTargetAddrFromURL_DoesNotMutate verifies that the caller's
// *url.URL is not modified by the helper (important — server.go reuses the
// parsed URL for forwarding).
func TestFormatTargetAddrFromURL_DoesNotMutate(t *testing.T) {
	original := "https://user:pass@api.vendor.com/v1?key=val#frag"
	u, err := url.Parse(original)
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}

	for _, mode := range []TargetAddrMode{TargetAddrModeHost, TargetAddrModePath, TargetAddrModeFull} {
		_ = FormatTargetAddrFromURL(u, mode)
	}

	if u.String() != original {
		t.Errorf("FormatTargetAddrFromURL mutated input URL: got %q, want %q", u.String(), original)
	}
	if u.User == nil || u.User.String() != "user:pass" {
		t.Errorf("FormatTargetAddrFromURL stripped userinfo from caller's URL: %v", u.User)
	}
	if u.RawQuery != "key=val" {
		t.Errorf("FormatTargetAddrFromURL stripped query from caller's URL: %q", u.RawQuery)
	}
	if u.Fragment != "frag" {
		t.Errorf("FormatTargetAddrFromURL stripped fragment from caller's URL: %q", u.Fragment)
	}
}

// TestFormatTargetAddrFromURL_EquivalentToParsing verifies that the URL and
// raw-string variants produce identical output for valid URLs. This is the
// invariant that lets the proxy use either form interchangeably.
func TestFormatTargetAddrFromURL_EquivalentToParsing(t *testing.T) {
	rawURLs := []string{
		"https://api.vendor.com/v1/users",
		"https://api.vendor.com:8443/v1/users?key=val",
		"https://user:pass@api.vendor.com/v1?token=abc#frag",
		"http://localhost:9999/echo",
	}
	modes := []TargetAddrMode{TargetAddrModeHost, TargetAddrModePath, TargetAddrModeFull}

	for _, raw := range rawURLs {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("url.Parse(%q) failed: %v", raw, err)
		}
		for _, mode := range modes {
			fromRaw := FormatTargetAddr(raw, mode)
			fromURL := FormatTargetAddrFromURL(u, mode)
			if fromRaw != fromURL {
				t.Errorf("FormatTargetAddr(%q, %q) = %q, but FormatTargetAddrFromURL(parsed, %q) = %q (must agree)",
					raw, mode, fromRaw, mode, fromURL)
			}
		}
	}
}
