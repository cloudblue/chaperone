// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"errors"
	"testing"
)

func TestNewAllowListValidator_EmptyAllowList(t *testing.T) {
	validator := NewAllowListValidator(nil)
	if validator == nil {
		t.Fatal("expected non-nil validator")
	}

	// Empty allow_list should deny all requests
	err := validator.Validate("https://api.google.com/v1")
	if err == nil {
		t.Error("expected error for empty allow_list")
	}
	if !errors.Is(err, ErrEmptyAllowList) {
		t.Errorf("expected ErrEmptyAllowList, got %v", err)
	}
}

func TestAllowListValidator_ValidHost(t *testing.T) {
	allowList := map[string][]string{
		"api.google.com": {"/v1/**", "/v2/**"},
	}

	validator := NewAllowListValidator(allowList)

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
		errType   error
	}{
		{
			name:      "valid host and path",
			targetURL: "https://api.google.com/v1/customers",
			wantErr:   false,
		},
		{
			name:      "valid host different path",
			targetURL: "https://api.google.com/v2/orders",
			wantErr:   false,
		},
		{
			name:      "valid host disallowed path",
			targetURL: "https://api.google.com/v3/admin",
			wantErr:   true,
			errType:   ErrPathNotAllowed,
		},
		{
			name:      "blocked host",
			targetURL: "https://evil.com/data",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

func TestAllowListValidator_DomainGlobs(t *testing.T) {
	allowList := map[string][]string{
		"*.google.com":      {"/**"},
		"**.amazonaws.com":  {"/bucket/**"},
		"exact.example.com": {"/v1/**"},
	}

	validator := NewAllowListValidator(allowList)

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
	}{
		{
			name:      "single star matches one subdomain",
			targetURL: "https://api.google.com/test",
			wantErr:   false,
		},
		{
			name:      "single star does not match two subdomains",
			targetURL: "https://a.b.google.com/test",
			wantErr:   true,
		},
		{
			name:      "double star matches multiple subdomains",
			targetURL: "https://a.b.c.amazonaws.com/bucket/file",
			wantErr:   false,
		},
		{
			name:      "double star matches single subdomain",
			targetURL: "https://s3.amazonaws.com/bucket/file",
			wantErr:   false,
		},
		{
			name:      "exact match works",
			targetURL: "https://exact.example.com/v1/users",
			wantErr:   false,
		},
		{
			name:      "exact match fails for subdomain",
			targetURL: "https://sub.exact.example.com/v1/users",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
		})
	}
}

func TestAllowListValidator_PathPatterns(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {
			"/v1/customers/*/profiles",
			"/v1/invoices/**",
			"/v2/**",
		},
	}

	validator := NewAllowListValidator(allowList)

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
	}{
		{
			name:      "single star matches one path segment",
			targetURL: "https://api.example.com/v1/customers/123/profiles",
			wantErr:   false,
		},
		{
			name:      "single star does not match nested path",
			targetURL: "https://api.example.com/v1/customers/123/456/profiles",
			wantErr:   true,
		},
		{
			name:      "double star matches nested path",
			targetURL: "https://api.example.com/v1/invoices/2024/01/inv-001",
			wantErr:   false,
		},
		{
			name:      "double star matches shallow path",
			targetURL: "https://api.example.com/v1/invoices/list",
			wantErr:   false,
		},
		{
			name:      "double star matches base path",
			targetURL: "https://api.example.com/v2",
			wantErr:   false,
		},
		{
			name:      "path not in allow list",
			targetURL: "https://api.example.com/admin/users",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
		})
	}
}

func TestAllowListValidator_URLEdgeCases(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	validator := NewAllowListValidator(allowList)

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
		errType   error
	}{
		{
			name:      "URL with explicit default https port",
			targetURL: "https://api.example.com:443/test",
			wantErr:   false,
		},
		{
			name:      "URL with non-standard port denied when allow-list host has no port",
			targetURL: "https://api.example.com:8443/test",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name:      "URL with query params - ignored in path matching",
			targetURL: "https://api.example.com/test?foo=bar&baz=qux",
			wantErr:   false,
		},
		{
			name:      "URL with fragment - ignored in path matching",
			targetURL: "https://api.example.com/test#section",
			wantErr:   false,
		},
		{
			name:      "empty path defaults to root",
			targetURL: "https://api.example.com",
			wantErr:   false,
		},
		{
			name:      "invalid URL returns error",
			targetURL: "://invalid",
			wantErr:   true,
			errType:   ErrInvalidTargetURL,
		},
		{
			name:      "empty URL returns error",
			targetURL: "",
			wantErr:   true,
			errType:   ErrInvalidTargetURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

func TestAllowListValidator_PortFilteringBehaviorMatrix(t *testing.T) {
	tests := []struct {
		name      string
		allowList map[string][]string
		targetURL string
		wantErr   bool
		errType   error
	}{
		{
			name: "no port in config allows implicit default https",
			allowList: map[string][]string{
				"api.vendor.com": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com/v1/x",
			wantErr:   false,
		},
		{
			name: "no port in config allows explicit default https",
			allowList: map[string][]string{
				"api.vendor.com": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com:443/v1/x",
			wantErr:   false,
		},
		{
			name: "no port in config denies non-standard https port",
			allowList: map[string][]string{
				"api.vendor.com": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com:8443/v1/x",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name: "explicit port in config allows exact match",
			allowList: map[string][]string{
				"api.vendor.com:8443": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com:8443/v1/x",
			wantErr:   false,
		},
		{
			name: "explicit port in config denies missing explicit port",
			allowList: map[string][]string{
				"api.vendor.com:8443": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com/v1/x",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name: "localhost explicit dev port allowed",
			allowList: map[string][]string{
				"localhost:8000": {"/api/v1/**"},
			},
			targetURL: "http://localhost:8000/api/v1",
			wantErr:   false,
		},
		{
			name: "localhost without port denies non-standard http port",
			allowList: map[string][]string{
				"localhost": {"/**"},
			},
			targetURL: "http://localhost:3000/test",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name: "localhost without port allows default http port",
			allowList: map[string][]string{
				"localhost": {"/**"},
			},
			targetURL: "http://localhost/test",
			wantErr:   false,
		},
		{
			name: "localhost without port allows explicit default http port",
			allowList: map[string][]string{
				"localhost": {"/**"},
			},
			targetURL: "http://localhost:80/test",
			wantErr:   false,
		},
		{
			name: "explicit port in config denies different explicit port",
			allowList: map[string][]string{
				"api.vendor.com:8443": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com:443/v1/x",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name: "wildcard host with explicit port allows exact port",
			allowList: map[string][]string{
				"*.vendor.com:8443": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com:8443/v1/x",
			wantErr:   false,
		},
		{
			name: "wildcard host with explicit port denies default port",
			allowList: map[string][]string{
				"*.vendor.com:8443": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com/v1/x",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name: "wildcard host without port denies non-default port",
			allowList: map[string][]string{
				"*.vendor.com": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com:9443/v1/x",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name: "wildcard host without port allows default port",
			allowList: map[string][]string{
				"*.vendor.com": {"/v1/**"},
			},
			targetURL: "https://api.vendor.com/v1/x",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewAllowListValidator(tt.allowList)
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

func TestAllowListValidator_CaseInsensitiveHost(t *testing.T) {
	allowList := map[string][]string{
		"api.google.com": {"/**"},
	}

	validator := NewAllowListValidator(allowList)

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
	}{
		{
			name:      "lowercase host matches",
			targetURL: "https://api.google.com/test",
			wantErr:   false,
		},
		{
			name:      "uppercase host matches",
			targetURL: "https://API.GOOGLE.COM/test",
			wantErr:   false,
		},
		{
			name:      "mixed case host matches",
			targetURL: "https://Api.Google.Com/test",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
		})
	}
}

func TestAllowListValidator_SecurityEdgeCases(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/v1/**"},
	}

	validator := NewAllowListValidator(allowList)

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
	}{
		{
			name:      "path traversal attempt blocked",
			targetURL: "https://api.example.com/v1/../admin/secret",
			wantErr:   true,
		},
		{
			name:      "encoded path traversal blocked",
			targetURL: "https://api.example.com/v1/%2e%2e/admin/secret",
			wantErr:   true,
		},
		{
			name:      "double encoded path traversal blocked",
			targetURL: "https://api.example.com/v1/%252e%252e/admin/secret",
			wantErr:   true,
		},
		{
			name:      "backslash path traversal blocked",
			targetURL: "https://api.example.com/v1/..\\admin\\secret",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAllowListConfig(t *testing.T) {
	tests := []struct {
		name      string
		allowList map[string][]string
		wantErr   bool
	}{
		{
			name: "valid config with exact host",
			allowList: map[string][]string{
				"api.google.com": {"/v1/**"},
			},
			wantErr: false,
		},
		{
			name: "valid config with domain glob",
			allowList: map[string][]string{
				"*.google.com": {"/v1/**"},
			},
			wantErr: false,
		},
		{
			name: "valid config with recursive domain glob",
			allowList: map[string][]string{
				"**.amazonaws.com": {"/bucket/**"},
			},
			wantErr: false,
		},
		{
			name: "invalid domain pattern - partial star",
			allowList: map[string][]string{
				"api*.google.com": {"/v1/**"},
			},
			wantErr: true,
		},
		{
			name: "invalid path pattern - partial star",
			allowList: map[string][]string{
				"api.google.com": {"/v1/cust*/profiles"},
			},
			wantErr: true,
		},
		{
			name: "invalid triple star in domain",
			allowList: map[string][]string{
				"***.google.com": {"/v1/**"},
			},
			wantErr: true,
		},
		{
			name: "invalid triple star in path",
			allowList: map[string][]string{
				"api.google.com": {"/v1/***/profiles"},
			},
			wantErr: true,
		},
		{
			name:      "nil allow list - valid but empty",
			allowList: nil,
			wantErr:   false,
		},
		{
			name:      "empty allow list",
			allowList: map[string][]string{},
			wantErr:   false,
		},
		{
			name: "empty path list for host",
			allowList: map[string][]string{
				"api.google.com": {},
			},
			wantErr: false, // Valid but will deny all paths for this host
		},
		{
			name: "valid host with explicit port",
			allowList: map[string][]string{
				"api.google.com:8443": {"/v1/**"},
			},
			wantErr: false,
		},
		{
			name: "valid host with wildcard and explicit port",
			allowList: map[string][]string{
				"*.google.com:443": {"/v1/**"},
			},
			wantErr: false,
		},
		{
			name: "invalid host port - non numeric",
			allowList: map[string][]string{
				"api.google.com:abc": {"/v1/**"},
			},
			wantErr: true,
		},
		{
			name: "invalid host port - out of range",
			allowList: map[string][]string{
				"api.google.com:70000": {"/v1/**"},
			},
			wantErr: true,
		},
		{
			name: "invalid host port - empty port separator",
			allowList: map[string][]string{
				"api.google.com:": {"/v1/**"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAllowListConfig(tt.allowList)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAllowListConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAllowListValidator_MoreSecurityCases tests additional security edge cases.
func TestAllowListValidator_MoreSecurityCases(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/v1/**"},
	}

	validator := NewAllowListValidator(allowList)

	tests := []struct {
		name      string
		targetURL string
		wantErr   bool
		errType   error
	}{
		// URL scheme attacks
		{
			name:      "missing scheme returns error",
			targetURL: "api.example.com/v1/test",
			wantErr:   true,
			errType:   ErrInvalidTargetURL,
		},
		{
			name:      "missing host returns error",
			targetURL: "https:///v1/test",
			wantErr:   true,
			errType:   ErrInvalidTargetURL,
		},
		// Path traversal variations
		{
			name:      "path traversal at start",
			targetURL: "https://api.example.com/../etc/passwd",
			wantErr:   true,
			errType:   ErrPathTraversalDetected,
		},
		{
			name:      "path traversal in middle",
			targetURL: "https://api.example.com/v1/a/../../../etc/passwd",
			wantErr:   true,
			errType:   ErrPathTraversalDetected,
		},
		{
			name:      "mixed encoding path traversal",
			targetURL: "https://api.example.com/v1/..%2f..%2fetc/passwd",
			wantErr:   true,
			errType:   ErrPathTraversalDetected,
		},
		{
			name:      "uppercase encoded path traversal",
			targetURL: "https://api.example.com/v1/%2E%2E/admin",
			wantErr:   true,
			errType:   ErrPathTraversalDetected,
		},
		// Host manipulation attempts
		{
			name:      "host with userinfo rejected",
			targetURL: "https://user:pass@api.example.com/v1/test",
			wantErr:   false, // URL parses, userinfo is stripped by Hostname()
		},
		// Unusual but valid URLs
		{
			name:      "ipv4 host not in allow list",
			targetURL: "https://192.168.1.1/v1/test",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		{
			name:      "ipv6 host not in allow list",
			targetURL: "https://[::1]/v1/test",
			wantErr:   true,
			errType:   ErrHostNotAllowed,
		},
		// Null byte in URL - Go's URL parser handles this, and it's a valid path char
		// (the null byte gets decoded but doesn't affect path matching)
		{
			name:      "null byte in path - allowed if path matches",
			targetURL: "https://api.example.com/v1/test%00.php",
			wantErr:   false, // Path /v1/test<null>.php matches /v1/**
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.targetURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.targetURL, err, tt.wantErr)
			}
			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

// TestAllowListValidator_PathTraversalVariations tests comprehensive path traversal detection.
func TestAllowListValidator_PathTraversalVariations(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	validator := NewAllowListValidator(allowList)

	// All these should be blocked as path traversal attempts
	traversalAttempts := []string{
		// Basic traversal
		"https://api.example.com/../secret",
		"https://api.example.com/a/../secret",
		"https://api.example.com/a/b/../../secret",
		// URL encoded
		"https://api.example.com/%2e%2e/secret",
		"https://api.example.com/%2e%2e%2fsecret",
		"https://api.example.com/%2E%2E/secret",
		// Double encoded
		"https://api.example.com/%252e%252e/secret",
		"https://api.example.com/%252e%252e%252fsecret",
		// Backslash variants (normalized to forward slash)
		"https://api.example.com/..\\secret",
		"https://api.example.com/a\\..\\secret",
		// Mixed
		"https://api.example.com/..%5csecret",
		"https://api.example.com/%2e%2e\\secret",
	}

	for _, url := range traversalAttempts {
		t.Run(url, func(t *testing.T) {
			err := validator.Validate(url)
			if err == nil {
				t.Errorf("expected path traversal to be blocked for %q", url)
			}
			if !errors.Is(err, ErrPathTraversalDetected) {
				t.Errorf("expected ErrPathTraversalDetected for %q, got %v", url, err)
			}
		})
	}
}

// TestAllowListValidator_ValidPathsNotBlockedAsTraversal ensures legitimate paths work.
func TestAllowListValidator_ValidPathsNotBlockedAsTraversal(t *testing.T) {
	allowList := map[string][]string{
		"api.example.com": {"/**"},
	}

	validator := NewAllowListValidator(allowList)

	// These should NOT be blocked
	validPaths := []string{
		"https://api.example.com/v1/test",
		"https://api.example.com/v1/..test",    // ".." not as segment
		"https://api.example.com/v1/test..txt", // ".." in filename
		"https://api.example.com/v1/a.b.c",
		"https://api.example.com/v1/file.tar.gz",
		"https://api.example.com/v1/path/to/resource",
		"https://api.example.com/v1/%20space%20in%20path",
		"https://api.example.com/v1/encoded%2Fslash", // %2F is /, but in filename
	}

	for _, url := range validPaths {
		t.Run(url, func(t *testing.T) {
			err := validator.Validate(url)
			if err != nil {
				t.Errorf("expected valid path %q to be allowed, got error: %v", url, err)
			}
		})
	}
}

// TestAllowListValidator_EmptyMapValidator tests behavior with empty (not nil) map.
func TestAllowListValidator_EmptyMapValidator(t *testing.T) {
	// Empty map should deny all (secure default)
	validator := NewAllowListValidator(map[string][]string{})

	err := validator.Validate("https://api.example.com/v1/test")
	if err == nil {
		t.Error("expected error for empty allow list, got nil")
	}
	if !errors.Is(err, ErrEmptyAllowList) {
		t.Errorf("expected ErrEmptyAllowList, got %v", err)
	}
}

// TestAllowListValidator_HostWithEmptyPaths tests host with empty path list.
func TestAllowListValidator_HostWithEmptyPaths(t *testing.T) {
	// Host in list but no paths allowed = deny all paths
	validator := NewAllowListValidator(map[string][]string{
		"api.example.com": {}, // Empty path list
	})

	err := validator.Validate("https://api.example.com/v1/test")
	if err == nil {
		t.Error("expected error for host with empty paths, got nil")
	}
	if !errors.Is(err, ErrPathNotAllowed) {
		t.Errorf("expected ErrPathNotAllowed, got %v", err)
	}
}

// FuzzAllowListValidator tests allow list validation with random inputs.
func FuzzAllowListValidator(f *testing.F) {
	// Add seed corpus
	seeds := []string{
		"https://api.example.com/v1/test",
		"https://evil.com/hack",
		"https://api.example.com/../secret",
		"https://api.example.com/%2e%2e/secret",
		"://invalid",
		"",
		"https://",
		"https://api.example.com",
		"https://api.example.com/",
		"https://API.EXAMPLE.COM/V1/TEST",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	// Create validator with common config
	validator := NewAllowListValidator(map[string][]string{
		"api.example.com": {"/v1/**", "/v2/**"},
		"*.google.com":    {"/**"},
	})

	f.Fuzz(func(t *testing.T, targetURL string) {
		// The function should not panic
		_ = validator.Validate(targetURL)
	})
}

// FuzzPathTraversalDetection specifically fuzzes the path traversal detection.
func FuzzPathTraversalDetection(f *testing.F) {
	// Add seed corpus of known attack patterns
	seeds := []string{
		"../secret",
		"..\\secret",
		"%2e%2e/secret",
		"%2e%2e%2fsecret",
		"%252e%252e/secret",
		"a/../b",
		"a/b/../../c",
		"/normal/path",
		"/path/with/..dots",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		// The function should not panic
		_ = checkPathTraversal(path)
	})
}
