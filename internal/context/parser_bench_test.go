// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/base64"
	"net/http/httptest"
	"runtime"
	"testing"
)

// Package-level sink to prevent compiler optimization of benchmark results.
var benchCtxSink any

// BenchmarkParseContext_MinimalHeaders benchmarks parsing with only required headers.
// Target: < 1us, 0 allocs
func BenchmarkParseContext_MinimalHeaders(b *testing.B) {
	req := httptest.NewRequest("POST", "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1/customers")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx, err := ParseContext(req, "X-Connect", "Connect-Request-ID")
		if err != nil {
			b.Fatal(err)
		}
		benchCtxSink = ctx
	}
}

// BenchmarkParseContext_AllHeaders benchmarks parsing with all standard headers.
// Target: < 1us, minimal allocs (Context-Data decoding requires allocations)
func BenchmarkParseContext_AllHeaders(b *testing.B) {
	contextData := base64.StdEncoding.EncodeToString([]byte(`{"key":"value","nested":{"a":"b"}}`))
	req := httptest.NewRequest("POST", "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1/customers")
	req.Header.Set("X-Connect-Environment-ID", "production")
	req.Header.Set("X-Connect-Marketplace-ID", "US")
	req.Header.Set("X-Connect-Vendor-ID", "microsoft")
	req.Header.Set("X-Connect-Product-ID", "product-123")
	req.Header.Set("X-Connect-Subscription-ID", "sub-456")
	req.Header.Set("X-Connect-Context-Data", contextData)
	req.Header.Set("Connect-Request-ID", "trace-abc-123")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx, err := ParseContext(req, "X-Connect", "Connect-Request-ID")
		if err != nil {
			b.Fatal(err)
		}
		benchCtxSink = ctx
	}
}

// BenchmarkParseContext_Parallel tests concurrent context parsing.
func BenchmarkParseContext_Parallel(b *testing.B) {
	contextData := base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`))

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1/customers")
			req.Header.Set("X-Connect-Vendor-ID", "microsoft")
			req.Header.Set("X-Connect-Context-Data", contextData)

			ctx, err := ParseContext(req, "X-Connect", "Connect-Request-ID")
			if err != nil {
				b.Error(err)
				return
			}
			runtime.KeepAlive(ctx)
		}
	})
}
