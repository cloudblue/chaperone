// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"runtime"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

// Package-level sink to prevent compiler optimization of benchmark results.
var benchHashSink string

// BenchmarkHashContext_Simple benchmarks hashing with minimal context.
// Target: < 5us, < 3 allocs
func BenchmarkHashContext_Simple(b *testing.B) {
	ctx := &sdk.TransactionContext{
		TargetURL:      "https://api.vendor.com/v1/customers",
		VendorID:       "microsoft",
		EnvironmentID:  "production",
		MarketplaceID:  "US",
		ProductID:      "product-123",
		SubscriptionID: "sub-456",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hash, err := HashContext(ctx)
		if err != nil {
			b.Fatal(err)
		}
		benchHashSink = hash
	}
}

// BenchmarkHashContext_WithData benchmarks hashing with Context-Data map.
// Target: < 10us, < 10 allocs (JSON marshaling requires allocations)
func BenchmarkHashContext_WithData(b *testing.B) {
	ctx := &sdk.TransactionContext{
		TargetURL:      "https://api.vendor.com/v1/customers",
		VendorID:       "microsoft",
		EnvironmentID:  "production",
		MarketplaceID:  "US",
		ProductID:      "product-123",
		SubscriptionID: "sub-456",
		Data: map[string]any{
			"key":    "value",
			"nested": map[string]any{"a": "b", "c": "d"},
			"array":  []any{1, 2, 3},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hash, err := HashContext(ctx)
		if err != nil {
			b.Fatal(err)
		}
		benchHashSink = hash
	}
}

// BenchmarkHashContext_NilContext benchmarks hashing with nil context (pre-computed return).
// Target: < 100ns, 0 allocs
func BenchmarkHashContext_NilContext(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hash, err := HashContext(nil)
		if err != nil {
			b.Fatal(err)
		}
		benchHashSink = hash
	}
}

// BenchmarkHashContext_Parallel tests concurrent hash computation.
// Safe: HashContext only reads from the TransactionContext (creates new sorted maps internally).
func BenchmarkHashContext_Parallel(b *testing.B) {
	ctx := &sdk.TransactionContext{
		TargetURL:      "https://api.vendor.com/v1/customers",
		VendorID:       "microsoft",
		EnvironmentID:  "production",
		MarketplaceID:  "US",
		ProductID:      "product-123",
		SubscriptionID: "sub-456",
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			hash, err := HashContext(ctx)
			if err != nil {
				b.Error(err)
				return
			}
			runtime.KeepAlive(hash)
		}
	})
}
