// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import "testing"

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips query string",
			input: "https://api.vendor.com/v1?api_key=secret",
			want:  "https://api.vendor.com/v1",
		},
		{
			name:  "strips fragment",
			input: "https://api.vendor.com/v1#section",
			want:  "https://api.vendor.com/v1",
		},
		{
			name:  "strips userinfo",
			input: "https://user:pass@api.vendor.com/v1",
			want:  "https://api.vendor.com/v1",
		},
		{
			name:  "preserves path",
			input: "https://api.vendor.com/v1/users/123",
			want:  "https://api.vendor.com/v1/users/123",
		},
		{
			name:  "strips all sensitive parts together",
			input: "https://user:pass@api.vendor.com/v1?token=abc#frag",
			want:  "https://api.vendor.com/v1",
		},
		{
			name:  "invalid URL returns empty",
			input: "://invalid",
			want:  "",
		},
		{
			name:  "empty URL returns empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURL(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
