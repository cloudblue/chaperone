// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"net/http"
	"testing"
)

// Package-level sink to prevent compiler optimization of benchmark results.
var benchReflectorSink bool

// BenchmarkReflector_ShouldStrip benchmarks header sensitivity check.
// Target: < 100ns, 0 allocs
func BenchmarkReflector_ShouldStrip(b *testing.B) {
	sensitiveHeaders := []string{
		"Authorization", "Proxy-Authorization", "Cookie",
		"Set-Cookie", "X-API-Key", "X-Auth-Token",
	}
	r := NewReflector(sensitiveHeaders)

	cases := []struct {
		name   string
		header string
	}{
		{"sensitive_exact", "Authorization"},
		{"sensitive_lowercase", "authorization"},
		{"sensitive_mixed", "X-API-Key"},
		{"not_sensitive", "Content-Type"},
		{"not_sensitive_long", "X-Custom-Header-Name"},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				benchReflectorSink = r.ShouldStrip(tc.header)
			}
		})
	}
}

// BenchmarkReflector_StripResponseHeaders benchmarks response header stripping.
// Target: < 1us, minimal allocs
// Note: Header construction inside the loop is constant overhead that cancels
// in regression comparisons. Avoids b.StopTimer()/b.StartTimer() which add
// more overhead than the operation being measured for sub-microsecond ops.
func BenchmarkReflector_StripResponseHeaders(b *testing.B) {
	sensitiveHeaders := []string{
		"Authorization", "Proxy-Authorization", "Cookie",
		"Set-Cookie", "X-API-Key", "X-Auth-Token",
	}
	r := NewReflector(sensitiveHeaders)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		headers := http.Header{
			"Content-Type":  {"application/json"},
			"Authorization": {"Bearer secret-token"},
			"X-Request-ID":  {"trace-123"},
			"Set-Cookie":    {"session=abc123"},
			"X-Custom":      {"value"},
		}
		r.StripResponseHeaders(headers)
	}
}

// BenchmarkStripInjectedHeaders benchmarks dynamic header stripping from context.
// Target: < 1us, minimal allocs
func BenchmarkStripInjectedHeaders(b *testing.B) {
	injectedKeys := []string{"X-Custom-Auth", "X-Vendor-Token", "X-Dynamic-Key"}
	ctx := WithInjectedHeaders(context.Background(), injectedKeys)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		headers := http.Header{
			"Content-Type":   {"application/json"},
			"X-Custom-Auth":  {"secret"},
			"X-Vendor-Token": {"token"},
			"X-Dynamic-Key":  {"key"},
			"X-Request-ID":   {"trace-123"},
		}
		StripInjectedHeaders(ctx, headers)
	}
}

// BenchmarkReflector_Parallel tests concurrent header stripping.
func BenchmarkReflector_Parallel(b *testing.B) {
	sensitiveHeaders := []string{
		"Authorization", "Proxy-Authorization", "Cookie",
		"Set-Cookie", "X-API-Key", "X-Auth-Token",
	}
	r := NewReflector(sensitiveHeaders)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			headers := http.Header{
				"Content-Type":  {"application/json"},
				"Authorization": {"Bearer secret-token"},
				"X-Request-ID":  {"trace-123"},
			}
			r.StripResponseHeaders(headers)
		}
	})
}
