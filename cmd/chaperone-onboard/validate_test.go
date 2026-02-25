// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
)

func TestValidateTenantID_ValidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tenant string
	}{
		{"GUID", "12345678-abcd-1234-abcd-1234567890ab"},
		{"domain", "contoso.onmicrosoft.com"},
		{"common", "common"},
		{"organizations", "organizations"},
		{"consumers", "consumers"},
		{"simple name", "contoso"},
		{"dotted name", "contoso.com"},
		{"hyphenated name", "my-tenant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateTenantID(tt.tenant); err != nil {
				t.Errorf("validateTenantID(%q) = %v, want nil", tt.tenant, err)
			}
		})
	}
}

func TestValidateTenantID_InvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tenant string
	}{
		{"empty", ""},
		{"path traversal", "../etc/passwd"},
		{"slash", "tenant/evil"},
		{"backslash", "tenant\\evil"},
		{"query injection", "tenant?q=evil"},
		{"fragment injection", "tenant#evil"},
		{"space", "tenant evil"},
		{"starts with dot", ".hidden"},
		{"starts with hyphen", "-bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateTenantID(tt.tenant); err == nil {
				t.Errorf("validateTenantID(%q) = nil, want error", tt.tenant)
			}
		})
	}
}

func TestValidateURL_ValidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{"standard HTTPS", "https://auth.example.com/authorize"},
		{"with port", "https://auth.example.com:8443/token"},
		{"with path", "https://login.microsoftonline.com/tenant/oauth2/authorize"},
		{"HTTP also accepted", "http://localhost:9999/token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateURL(tt.url); err != nil {
				t.Errorf("validateURL(%q) = %v, want nil", tt.url, err)
			}
		})
	}
}

func TestValidateURL_InvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"no scheme", "auth.example.com/authorize"},
		{"ftp scheme", "ftp://auth.example.com/authorize"},
		{"no host", "https:///path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateURL(tt.url); err == nil {
				t.Errorf("validateURL(%q) = nil, want error", tt.url)
			}
		})
	}
}

func TestIsHTTPS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com", true},
		{"http://example.com", false},
		{"", false},
		{"not-a-url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			if got := isHTTPS(tt.url); got != tt.want {
				t.Errorf("isHTTPS(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestValidateNonEmpty_Valid(t *testing.T) {
	t.Parallel()

	if err := validateNonEmpty("client-id", "my-app"); err != nil {
		t.Errorf("validateNonEmpty(non-empty) = %v, want nil", err)
	}
}

func TestValidateNonEmpty_Empty(t *testing.T) {
	t.Parallel()

	err := validateNonEmpty("client-id", "")
	if err == nil {
		t.Error("validateNonEmpty(empty) = nil, want error")
	}
	if err != nil && err.Error() != "client-id is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}
