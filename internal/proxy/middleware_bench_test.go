// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkPanicRecoveryMiddleware benchmarks the panic recovery wrapper overhead.
// Target: < 500ns overhead per request
func BenchmarkPanicRecoveryMiddleware(b *testing.B) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WithPanicRecovery(inner)
	req := httptest.NewRequest("GET", "/proxy", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkRequestLoggingMiddleware benchmarks the request logging wrapper overhead.
// Logs are silenced to measure middleware overhead without I/O bias.
func BenchmarkRequestLoggingMiddleware(b *testing.B) {
	silenceLogs(b)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WithRequestLogging("Connect-Request-ID", inner)
	req := httptest.NewRequest("GET", "/proxy", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkMiddlewareStack benchmarks the combined middleware stack.
// Target: < 50us total overhead (excluding upstream)
func BenchmarkMiddlewareStack(b *testing.B) {
	silenceLogs(b)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Stack middlewares as they would be in production
	handler := WithPanicRecovery(WithRequestLogging("Connect-Request-ID", inner))
	req := httptest.NewRequest("GET", "/proxy", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkMiddlewareStack_Parallel tests concurrent middleware execution.
func BenchmarkMiddlewareStack_Parallel(b *testing.B) {
	silenceLogs(b)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WithPanicRecovery(WithRequestLogging("Connect-Request-ID", inner))

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/proxy", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}
	})
}
