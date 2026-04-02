// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import "testing"

func TestExtractTargetHost(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "returns host only, strips path",
			input: "https://api.vendor.com/v1/users/123",
			want:  "api.vendor.com",
		},
		{
			name:  "returns host only, strips query string",
			input: "https://api.vendor.com/v1?api_key=secret",
			want:  "api.vendor.com",
		},
		{
			name:  "returns host only, strips userinfo",
			input: "https://user:pass@api.vendor.com/v1",
			want:  "api.vendor.com",
		},
		{
			name:  "preserves port",
			input: "https://api.vendor.com:8443/v1",
			want:  "api.vendor.com:8443",
		},
		{
			name:  "strips all sensitive parts together",
			input: "https://user:pass@api.vendor.com/v1/users/alice@example.com?token=abc#frag",
			want:  "api.vendor.com",
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
			got := extractTargetHost(tt.input)
			if got != tt.want {
				t.Errorf("extractTargetHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
