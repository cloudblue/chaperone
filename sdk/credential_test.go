// Copyright 2024-2026 CloudBlue
// SPDX-License-Identifier: Apache-2.0

package sdk

import (
	"testing"
	"time"
)

func TestCredential_IsExpired_NilCredential_ReturnsTrue(t *testing.T) {
	var c *Credential
	if !c.IsExpired() {
		t.Error("nil credential should be considered expired")
	}
}

func TestCredential_IsExpired_ExpiredCredential_ReturnsTrue(t *testing.T) {
	c := &Credential{
		Headers:   map[string]string{"Authorization": "Bearer token"},
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}
	if !c.IsExpired() {
		t.Error("credential with past ExpiresAt should be expired")
	}
}

func TestCredential_IsExpired_ValidCredential_ReturnsFalse(t *testing.T) {
	c := &Credential{
		Headers:   map[string]string{"Authorization": "Bearer token"},
		ExpiresAt: time.Now().Add(1 * time.Hour), // Expires in 1 hour
	}
	if c.IsExpired() {
		t.Error("credential with future ExpiresAt should not be expired")
	}
}

func TestCredential_IsExpired_JustExpired_ReturnsTrue(t *testing.T) {
	// Credential that expired 1 millisecond ago
	c := &Credential{
		Headers:   map[string]string{"Authorization": "Bearer token"},
		ExpiresAt: time.Now().Add(-1 * time.Millisecond),
	}
	if !c.IsExpired() {
		t.Error("credential just past ExpiresAt should be expired")
	}
}

func TestCredential_TTL_NilCredential_ReturnsZero(t *testing.T) {
	var c *Credential
	ttl := c.TTL()
	if ttl != 0 {
		t.Errorf("nil credential TTL should be 0, got %v", ttl)
	}
}

func TestCredential_TTL_ExpiredCredential_ReturnsZero(t *testing.T) {
	c := &Credential{
		Headers:   map[string]string{"Authorization": "Bearer token"},
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	ttl := c.TTL()
	if ttl != 0 {
		t.Errorf("expired credential TTL should be 0, got %v", ttl)
	}
}

func TestCredential_TTL_ValidCredential_ReturnsPositiveDuration(t *testing.T) {
	expectedTTL := 1 * time.Hour
	c := &Credential{
		Headers:   map[string]string{"Authorization": "Bearer token"},
		ExpiresAt: time.Now().Add(expectedTTL),
	}

	ttl := c.TTL()

	// Allow some tolerance for test execution time
	if ttl <= 0 {
		t.Error("valid credential should have positive TTL")
	}
	if ttl > expectedTTL {
		t.Errorf("TTL %v should not exceed expected %v", ttl, expectedTTL)
	}
	// Should be close to expected (within 1 second of expected)
	if expectedTTL-ttl > time.Second {
		t.Errorf("TTL %v is too far from expected %v", ttl, expectedTTL)
	}
}

func TestCredential_TTL_ShortTTL_ReturnsAccurateValue(t *testing.T) {
	// Test with a short TTL to verify accuracy
	expectedTTL := 5 * time.Second
	c := &Credential{
		Headers:   map[string]string{"X-API-Key": "key123"},
		ExpiresAt: time.Now().Add(expectedTTL),
	}

	ttl := c.TTL()

	// Should be within 100ms of expected (accounting for test execution)
	tolerance := 100 * time.Millisecond
	if ttl < expectedTTL-tolerance || ttl > expectedTTL {
		t.Errorf("TTL %v should be close to %v (±%v)", ttl, expectedTTL, tolerance)
	}
}

func TestCredential_IsExpired_TableDriven(t *testing.T) {
	tests := []struct {
		expiresAt   time.Time
		name        string
		wantExpired bool
	}{
		{
			name:        "future expiration",
			expiresAt:   time.Now().Add(24 * time.Hour),
			wantExpired: false,
		},
		{
			name:        "past expiration",
			expiresAt:   time.Now().Add(-24 * time.Hour),
			wantExpired: true,
		},
		{
			name:        "just expired (1 second ago)",
			expiresAt:   time.Now().Add(-1 * time.Second),
			wantExpired: true,
		},
		{
			name:        "about to expire (1 second from now)",
			expiresAt:   time.Now().Add(1 * time.Second),
			wantExpired: false,
		},
		{
			name:        "zero time",
			expiresAt:   time.Time{},
			wantExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Credential{
				Headers:   map[string]string{"Authorization": "Bearer test"},
				ExpiresAt: tt.expiresAt,
			}
			if got := c.IsExpired(); got != tt.wantExpired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.wantExpired)
			}
		})
	}
}

func TestCredential_TTL_TableDriven(t *testing.T) {
	tests := []struct {
		credential *Credential
		name       string
		minTTL     time.Duration
		maxTTL     time.Duration
		wantZero   bool
	}{
		{
			name:       "nil credential",
			credential: nil,
			wantZero:   true,
		},
		{
			name: "expired credential",
			credential: &Credential{
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			wantZero: true,
		},
		{
			name: "1 hour TTL",
			credential: &Credential{
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			wantZero: false,
			minTTL:   59 * time.Minute,
			maxTTL:   1 * time.Hour,
		},
		{
			name: "1 minute TTL",
			credential: &Credential{
				ExpiresAt: time.Now().Add(1 * time.Minute),
			},
			wantZero: false,
			minTTL:   59 * time.Second,
			maxTTL:   1 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.credential.TTL()

			if tt.wantZero {
				if got != 0 {
					t.Errorf("TTL() = %v, want 0", got)
				}
				return
			}

			if got < tt.minTTL || got > tt.maxTTL {
				t.Errorf("TTL() = %v, want between %v and %v", got, tt.minTTL, tt.maxTTL)
			}
		})
	}
}

// TestCredential_Headers_InjectedCorrectly verifies the credential structure
// works as expected for header injection.
func TestCredential_Headers_InjectedCorrectly(t *testing.T) {
	c := &Credential{
		Headers: map[string]string{
			"Authorization": "Bearer abc123",
			"X-Custom":      "custom-value",
		},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	// Verify headers are accessible
	if c.Headers["Authorization"] != "Bearer abc123" {
		t.Errorf("Authorization header = %q, want %q", c.Headers["Authorization"], "Bearer abc123")
	}
	if c.Headers["X-Custom"] != "custom-value" {
		t.Errorf("X-Custom header = %q, want %q", c.Headers["X-Custom"], "custom-value")
	}

	// Verify non-expired
	if c.IsExpired() {
		t.Error("credential should not be expired")
	}

	// Verify TTL is positive
	if c.TTL() <= 0 {
		t.Error("credential should have positive TTL")
	}
}
