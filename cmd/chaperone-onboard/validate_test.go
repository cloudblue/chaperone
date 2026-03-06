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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateURL(tt.url, false); err != nil {
				t.Errorf("validateURL(%q, false) = %v, want nil", tt.url, err)
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
		{"HTTP rejected by default", "http://localhost:9999/token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateURL(tt.url, false); err == nil {
				t.Errorf("validateURL(%q, false) = nil, want error", tt.url)
			}
		})
	}
}

func TestValidateURL_AllowHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"HTTPS still valid", "https://auth.example.com/authorize", false},
		{"HTTP accepted", "http://localhost:9999/token", false},
		{"FTP still rejected", "ftp://auth.example.com/authorize", true},
		{"empty still rejected", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateURL(tt.url, true)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q, true) error = %v, wantErr %v", tt.url, err, tt.wantErr)
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

func TestParseExtraParams_Valid(t *testing.T) {
	t.Parallel()

	v, err := parseExtraParams("a=1,b=2")
	if err != nil {
		t.Fatalf("parseExtraParams() error = %v", err)
	}
	assertParam(t, v, "a", "1")
	assertParam(t, v, "b", "2")
}

func TestParseExtraParams_EmptyValue(t *testing.T) {
	t.Parallel()

	v, err := parseExtraParams("key=")
	if err != nil {
		t.Fatalf("parseExtraParams() error = %v", err)
	}
	assertParam(t, v, "key", "")
}

func TestParseExtraParams_TrailingComma(t *testing.T) {
	t.Parallel()

	v, err := parseExtraParams("key=value,")
	if err != nil {
		t.Fatalf("parseExtraParams() error = %v", err)
	}
	assertParam(t, v, "key", "value")
}

func TestParseExtraParams_InvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"no equals", "keyonly"},
		{"empty key", "=value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseExtraParams(tt.input); err == nil {
				t.Errorf("parseExtraParams(%q) = nil, want error", tt.input)
			}
		})
	}
}
