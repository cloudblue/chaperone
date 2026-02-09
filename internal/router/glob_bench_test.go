// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"runtime"
	"testing"
)

// Package-level sink to prevent compiler optimization of benchmark results.
var benchGlobSink bool

// BenchmarkGlobMatch benchmarks different glob pattern types.
// Target: < 10us per pattern, 0 allocs
func BenchmarkGlobMatch(b *testing.B) {
	cases := []struct {
		name    string
		pattern string
		input   string
		sep     byte
	}{
		{"exact_match_host", "api.vendor.com", "api.vendor.com", '.'},
		{"exact_match_path", "/v1/customers", "/v1/customers", '/'},
		{"single_star_host", "*.vendor.com", "api.vendor.com", '.'},
		{"single_star_path", "/v1/*/profiles", "/v1/customers/profiles", '/'},
		{"double_star_host", "**.amazonaws.com", "s3.us-east-1.amazonaws.com", '.'},
		{"double_star_path", "/v1/**", "/v1/customers/123/orders/456", '/'},
		{"no_match_host", "*.google.com", "api.vendor.com", '.'},
		{"no_match_path", "/v1/users/**", "/v2/customers/123", '/'},
		{"complex_path", "/v1/customers/*/orders/**", "/v1/customers/123/orders/456/items", '/'},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				benchGlobSink = GlobMatch(tc.pattern, tc.input, tc.sep)
			}
		})
	}
}

// BenchmarkGlobMatch_Parallel tests concurrent glob matching.
func BenchmarkGlobMatch_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result := GlobMatch("/v1/**", "/v1/customers/123/orders/456", '/')
			runtime.KeepAlive(result)
		}
	})
}
