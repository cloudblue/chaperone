// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudblue/chaperone/internal/config"
)

// testSensitiveHeaders returns the default sensitive headers for testing.
func testSensitiveHeaders() []string {
	return config.MergeSensitiveHeaders(nil)
}

func TestReflector_StripResponseHeaders_RemovesSensitiveHeaders(t *testing.T) {
	tests := []struct {
		name           string
		responseHeader http.Header
		wantRemoved    []string // Headers that should be removed
		wantKept       []string // Headers that should remain
	}{
		{
			name: "authorization stripped from response",
			responseHeader: http.Header{
				"Authorization": []string{"Bearer leaked-token"},
				"Content-Type":  []string{"application/json"},
			},
			wantRemoved: []string{"Authorization"},
			wantKept:    []string{"Content-Type"},
		},
		{
			name: "all sensitive headers stripped",
			responseHeader: http.Header{
				"Authorization":       []string{"Bearer token"},
				"Proxy-Authorization": []string{"Basic creds"},
				"Cookie":              []string{"session=abc"},
				"Set-Cookie":          []string{"token=xyz"},
				"X-Api-Key":           []string{"key"},
				"X-Auth-Token":        []string{"token"},
				"Content-Length":      []string{"100"},
				"X-Request-Id":        []string{"123"},
			},
			wantRemoved: []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-Api-Key", "X-Auth-Token"},
			wantKept:    []string{"Content-Length", "X-Request-Id"},
		},
		{
			name: "no sensitive headers unchanged",
			responseHeader: http.Header{
				"Content-Type":  []string{"application/json"},
				"X-Request-Id":  []string{"abc123"},
				"Cache-Control": []string{"no-cache"},
			},
			wantRemoved: []string{},
			wantKept:    []string{"Content-Type", "X-Request-Id", "Cache-Control"},
		},
		{
			name:           "empty headers",
			responseHeader: http.Header{},
			wantRemoved:    []string{},
			wantKept:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			reflector := NewReflector(testSensitiveHeaders())

			// Act - reflector modifies in place
			reflector.StripResponseHeaders(tt.responseHeader)

			// Assert - removed headers
			for _, h := range tt.wantRemoved {
				if tt.responseHeader.Get(h) != "" {
					t.Errorf("header %q should have been removed", h)
				}
			}

			// Assert - kept headers
			for _, h := range tt.wantKept {
				if tt.responseHeader.Get(h) == "" {
					t.Errorf("header %q should have been kept", h)
				}
			}
		})
	}
}

func TestReflector_StripResponseHeaders_CaseInsensitive(t *testing.T) {
	// Arrange
	reflector := NewReflector(testSensitiveHeaders())
	headers := http.Header{
		"authorization": []string{"token"},
		"COOKIE":        []string{"session"},
	}

	// Act
	reflector.StripResponseHeaders(headers)

	// Assert - Go's http.Header normalizes case, so these should be removed
	if headers.Get("Authorization") != "" {
		t.Error("Authorization should be removed (case insensitive)")
	}
	if headers.Get("Cookie") != "" {
		t.Error("Cookie should be removed (case insensitive)")
	}
}

func TestReflector_CustomHeaders(t *testing.T) {
	// Arrange
	customHeaders := []string{"X-Custom-Secret"}
	reflector := NewReflector(customHeaders)
	headers := http.Header{
		"X-Custom-Secret": []string{"secret"},
		"Authorization":   []string{"token"}, // NOT in custom list
	}

	// Act
	reflector.StripResponseHeaders(headers)

	// Assert
	if headers.Get("X-Custom-Secret") != "" {
		t.Error("X-Custom-Secret should be removed")
	}
	if headers.Get("Authorization") == "" {
		t.Error("Authorization should NOT be removed with custom list")
	}
}

func TestReflector_ShouldStrip(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{"authorization", "Authorization", true},
		{"cookie", "Cookie", true},
		{"set-cookie", "Set-Cookie", true},
		{"content-type not stripped", "Content-Type", false},
		{"x-request-id not stripped", "X-Request-ID", false},
	}

	reflector := NewReflector(testSensitiveHeaders())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reflector.ShouldStrip(tt.header)
			if got != tt.expected {
				t.Errorf("ShouldStrip(%q) = %v, want %v", tt.header, got, tt.expected)
			}
		})
	}
}

func TestReflector_StripResponseHeaders_ModifiesInPlace(t *testing.T) {
	// Arrange
	reflector := NewReflector(testSensitiveHeaders())
	headers := http.Header{
		"Authorization": []string{"Bearer token"},
	}

	// Act
	reflector.StripResponseHeaders(headers)

	// Assert - the original headers should be modified
	if len(headers) != 0 {
		t.Error("StripResponseHeaders should modify headers in place")
	}
}

// --- Dynamic Injection Header Stripping Tests ---

func TestWithInjectedHeaders_StoresAndRetrieves(t *testing.T) {
	ctx := context.Background()
	keys := []string{"X-Vendor-Token", "X-Custom-Auth"}

	ctx = WithInjectedHeaders(ctx, keys)
	got := InjectedHeaders(ctx)

	if len(got) != len(keys) {
		t.Fatalf("InjectedHeaders() returned %d keys, want %d", len(got), len(keys))
	}
	for i, k := range keys {
		if got[i] != k {
			t.Errorf("InjectedHeaders()[%d] = %q, want %q", i, got[i], k)
		}
	}
}

func TestInjectedHeaders_EmptyContext_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	got := InjectedHeaders(ctx)
	if got != nil {
		t.Errorf("InjectedHeaders() on empty context = %v, want nil", got)
	}
}

func TestStripInjectedHeaders_StripsInjectedKeys(t *testing.T) {
	// Arrange — simulate plugin injecting X-Vendor-Magic-Token
	ctx := context.Background()
	ctx = WithInjectedHeaders(ctx, []string{"X-Vendor-Magic-Token"})

	// ISV echoes the injected header back in its response
	headers := http.Header{
		"X-Vendor-Magic-Token": []string{"secret123"},
		"Content-Type":         []string{"application/json"},
	}

	// Act
	StripInjectedHeaders(ctx, headers)

	// Assert
	if headers.Get("X-Vendor-Magic-Token") != "" {
		t.Error("X-Vendor-Magic-Token should have been stripped from response")
	}
	if headers.Get("Content-Type") == "" {
		t.Error("Content-Type should be preserved")
	}
}

func TestStripInjectedHeaders_CaseInsensitive(t *testing.T) {
	// http.Header.Del is case-insensitive via textproto.CanonicalMIMEHeaderKey
	ctx := context.Background()
	ctx = WithInjectedHeaders(ctx, []string{"x-vendor-token"})

	headers := http.Header{
		"X-Vendor-Token": []string{"secret"},
	}

	StripInjectedHeaders(ctx, headers)

	if headers.Get("X-Vendor-Token") != "" {
		t.Error("injected header should be stripped case-insensitively")
	}
}

func TestStripInjectedHeaders_NoContext_NoOp(t *testing.T) {
	ctx := context.Background()
	headers := http.Header{
		"Authorization": []string{"Bearer token"},
		"Content-Type":  []string{"application/json"},
	}

	// Act — no injected headers in context
	StripInjectedHeaders(ctx, headers)

	// Assert — nothing removed
	if headers.Get("Authorization") == "" {
		t.Error("no headers should be removed when context has no injected keys")
	}
}

func TestStripInjectedHeaders_MultipleKeys(t *testing.T) {
	ctx := context.Background()
	ctx = WithInjectedHeaders(ctx, []string{
		"X-Vendor-Token",
		"X-Custom-Signature",
		"X-Api-Nonce",
	})

	headers := http.Header{
		"X-Vendor-Token":     []string{"token"},
		"X-Custom-Signature": []string{"sig"},
		"X-Api-Nonce":        []string{"nonce"},
		"Content-Type":       []string{"application/json"},
		"X-Request-Id":       []string{"abc"},
	}

	StripInjectedHeaders(ctx, headers)

	for _, key := range []string{"X-Vendor-Token", "X-Custom-Signature", "X-Api-Nonce"} {
		if headers.Get(key) != "" {
			t.Errorf("injected header %q should have been stripped", key)
		}
	}
	for _, key := range []string{"Content-Type", "X-Request-Id"} {
		if headers.Get(key) == "" {
			t.Errorf("non-injected header %q should be preserved", key)
		}
	}
}

func TestStripInjectedHeaders_CombinedWithStaticStripping(t *testing.T) {
	// Scenario: static reflector strips Authorization, dynamic strips X-Vendor-Token
	reflector := NewReflector(testSensitiveHeaders())
	ctx := context.Background()
	ctx = WithInjectedHeaders(ctx, []string{"X-Vendor-Magic-Token"})

	headers := http.Header{
		"Authorization":        []string{"Bearer leaked"},
		"X-Vendor-Magic-Token": []string{"vendor-secret"},
		"Content-Type":         []string{"application/json"},
	}

	// Static stripping (well-known headers)
	reflector.StripResponseHeaders(headers)
	// Dynamic stripping (per-request injected headers)
	StripInjectedHeaders(ctx, headers)

	if headers.Get("Authorization") != "" {
		t.Error("Authorization should be stripped by static reflector")
	}
	if headers.Get("X-Vendor-Magic-Token") != "" {
		t.Error("X-Vendor-Magic-Token should be stripped by dynamic injection stripping")
	}
	if headers.Get("Content-Type") == "" {
		t.Error("Content-Type should be preserved")
	}
}

func TestStripInjectedHeaders_HeaderNotInResponse_NoOp(t *testing.T) {
	// Injected header wasn't echoed back by ISV — should be a safe no-op
	ctx := context.Background()
	ctx = WithInjectedHeaders(ctx, []string{"X-Vendor-Token"})

	headers := http.Header{
		"Content-Type": []string{"application/json"},
	}

	StripInjectedHeaders(ctx, headers)

	if headers.Get("Content-Type") == "" {
		t.Error("Content-Type should be preserved")
	}
}
