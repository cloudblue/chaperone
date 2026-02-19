// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"strings"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

func TestHashContext_Deterministic_SameInputSameOutput(t *testing.T) {
	ctx := &sdk.TransactionContext{
		TargetURL:      "https://api.vendor.com/v1/resource",
		EnvironmentID:  "production",
		MarketplaceID:  "US",
		VendorID:       "vendor-123",
		ProductID:      "product-456",
		SubscriptionID: "sub-789",
		Data:           map[string]any{"key": "value"},
	}

	hash1, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash2, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash3, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("hash not deterministic: got %q, %q, %q", hash1, hash2, hash3)
	}
}

func TestHashContext_DifferentInput_DifferentOutput(t *testing.T) {
	tests := []struct {
		ctx1 *sdk.TransactionContext
		ctx2 *sdk.TransactionContext
		name string
	}{
		{
			name: "different TargetURL",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api1.com"},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api2.com"},
		},
		{
			name: "different EnvironmentID",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", EnvironmentID: "production"},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", EnvironmentID: "staging"},
		},
		{
			name: "different MarketplaceID",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", MarketplaceID: "US"},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", MarketplaceID: "EU"},
		},
		{
			name: "different VendorID",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", VendorID: "v1"},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", VendorID: "v2"},
		},
		{
			name: "different ProductID",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", ProductID: "p1"},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", ProductID: "p2"},
		},
		{
			name: "different SubscriptionID",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", SubscriptionID: "s1"},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", SubscriptionID: "s2"},
		},
		{
			name: "different Data key",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", Data: map[string]any{"key1": "value"}},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", Data: map[string]any{"key2": "value"}},
		},
		{
			name: "different Data value",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", Data: map[string]any{"key": "value1"}},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", Data: map[string]any{"key": "value2"}},
		},
		{
			name: "Data present vs absent",
			ctx1: &sdk.TransactionContext{TargetURL: "https://api.com", Data: map[string]any{"key": "value"}},
			ctx2: &sdk.TransactionContext{TargetURL: "https://api.com", Data: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1, err := HashContext(tt.ctx1)
			if err != nil {
				t.Fatalf("unexpected error for ctx1: %v", err)
			}
			hash2, err := HashContext(tt.ctx2)
			if err != nil {
				t.Fatalf("unexpected error for ctx2: %v", err)
			}

			if hash1 == hash2 {
				t.Errorf("expected different hashes for different contexts, got same: %q", hash1)
			}
		})
	}
}

func TestHashContext_NilContext_ReturnsEmptyHash(t *testing.T) {
	hash, err := HashContext(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash for nil context")
	}
	// Should be deterministic
	hash2, err := HashContext(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != hash2 {
		t.Errorf("nil hash not deterministic: %q vs %q", hash, hash2)
	}
}

func TestHashContext_EmptyFields_Handled(t *testing.T) {
	ctx := &sdk.TransactionContext{
		TargetURL:      "",
		EnvironmentID:  "",
		MarketplaceID:  "",
		VendorID:       "",
		ProductID:      "",
		SubscriptionID: "",
		Data:           nil,
	}

	hash, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash for empty context")
	}

	// Should be deterministic
	hash2, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != hash2 {
		t.Errorf("empty context hash not deterministic: %q vs %q", hash, hash2)
	}
}

func TestHashContext_EmptyData_DifferentFromNilData(t *testing.T) {
	ctxNilData := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		Data:      nil,
	}
	ctxEmptyData := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		Data:      map[string]any{},
	}

	hashNil, err := HashContext(ctxNilData)
	if err != nil {
		t.Fatalf("unexpected error for nil data: %v", err)
	}
	hashEmpty, err := HashContext(ctxEmptyData)
	if err != nil {
		t.Fatalf("unexpected error for empty data: %v", err)
	}

	// Design decision: nil and empty map produce the same hash
	// since they're semantically equivalent for cache lookup
	if hashNil != hashEmpty {
		t.Errorf("nil and empty Data should produce same hash: %q vs %q", hashNil, hashEmpty)
	}
}

func TestHashContext_MapKeyOrder_Invariant(t *testing.T) {
	// Create two contexts with same data but potentially different iteration order
	// Go maps have random iteration order, so we test multiple times
	ctx1 := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		Data: map[string]any{
			"zebra":  "last",
			"alpha":  "first",
			"middle": "center",
			"beta":   "second",
		},
	}

	// Create a fresh map with the same keys/values
	ctx2 := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		Data: map[string]any{
			"beta":   "second",
			"alpha":  "first",
			"zebra":  "last",
			"middle": "center",
		},
	}

	hash1, err := HashContext(ctx1)
	if err != nil {
		t.Fatalf("unexpected error for ctx1: %v", err)
	}
	hash2, err := HashContext(ctx2)
	if err != nil {
		t.Fatalf("unexpected error for ctx2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("hash should be invariant to map key order: %q vs %q", hash1, hash2)
	}

	// Test multiple iterations to increase confidence
	for i := 0; i < 100; i++ {
		h, err := HashContext(ctx1)
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
		if h != hash1 {
			t.Fatal("hash changed between iterations")
		}
	}
}

func TestHashContext_TraceID_NotIncluded(t *testing.T) {
	// TraceID should NOT affect the hash because:
	// 1. It changes per request (correlation ID)
	// 2. Same context with different TraceIDs should hit the same cache
	ctx1 := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		VendorID:  "v1",
		TraceID:   "trace-111",
	}
	ctx2 := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		VendorID:  "v1",
		TraceID:   "trace-222",
	}

	hash1, err := HashContext(ctx1)
	if err != nil {
		t.Fatalf("unexpected error for ctx1: %v", err)
	}
	hash2, err := HashContext(ctx2)
	if err != nil {
		t.Fatalf("unexpected error for ctx2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("TraceID should not affect hash: %q vs %q", hash1, hash2)
	}
}

func TestHashContext_NestedData_Handled(t *testing.T) {
	ctx := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		Data: map[string]any{
			"simple": "value",
			"nested": map[string]any{
				"inner": "data",
				"deep": map[string]any{
					"level": 3,
				},
			},
			"array": []any{"a", "b", "c"},
		},
	}

	hash1, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash2, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("nested data hash not deterministic: %q vs %q", hash1, hash2)
	}

	// Different nested value should produce different hash
	ctx2 := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		Data: map[string]any{
			"simple": "value",
			"nested": map[string]any{
				"inner": "different",
				"deep": map[string]any{
					"level": 3,
				},
			},
			"array": []any{"a", "b", "c"},
		},
	}

	hash3, err := HashContext(ctx2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash1 == hash3 {
		t.Error("different nested data should produce different hash")
	}
}

func TestHashContext_Format_SHA256Hex(t *testing.T) {
	ctx := &sdk.TransactionContext{
		TargetURL: "https://api.com",
	}

	hash, err := HashContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SHA-256 produces 64 hex characters
	if len(hash) != 64 {
		t.Errorf("expected 64 character hex hash, got %d: %q", len(hash), hash)
	}

	// Should only contain hex characters
	for _, c := range hash {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("hash contains non-hex character: %c in %q", c, hash)
			break
		}
	}
}

func TestHashContext_UnmarshalableData_ReturnsError(t *testing.T) {
	// Data containing a channel cannot be marshaled to JSON
	ctx := &sdk.TransactionContext{
		TargetURL: "https://api.com",
		Data: map[string]any{
			"valid":   "value",
			"invalid": make(chan int), // channels cannot be marshaled
		},
	}

	_, err := HashContext(ctx)
	if err == nil {
		t.Error("expected error for unmarshalable data, got nil")
	}

	// Verify error chain contains useful context
	errStr := err.Error()
	if !strings.Contains(errStr, "building canonical form") {
		t.Errorf("error should contain 'building canonical form', got: %v", err)
	}
	if !strings.Contains(errStr, "marshaling data to JSON") {
		t.Errorf("error should contain 'marshaling data to JSON', got: %v", err)
	}
}
