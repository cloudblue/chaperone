// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

// BenchmarkRedactingHandler_NoSecrets benchmarks log handling without secrets in context.
// Target: < 2us, minimal allocs beyond inner handler
func BenchmarkRedactingHandler_NoSecrets(b *testing.B) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	sensitiveHeaders := []string{"Authorization", "Cookie", "X-API-Key"}
	handler := NewRedactingHandler(inner, sensitiveHeaders, false)
	logger := slog.New(handler)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.InfoContext(ctx, "request processed",
			"trace_id", "abc-123",
			"status", 200,
			"latency_ms", 42,
		)
	}
}

// BenchmarkRedactingHandler_WithSecrets benchmarks log handling with secrets in context.
// Target: < 5us
func BenchmarkRedactingHandler_WithSecrets(b *testing.B) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	sensitiveHeaders := []string{"Authorization", "Cookie", "X-API-Key"}
	handler := NewRedactingHandler(inner, sensitiveHeaders, false)
	logger := slog.New(handler)

	ctx := context.Background()
	ctx = WithSecretValue(ctx, "Bearer super-secret-token-12345")
	ctx = WithSecretValue(ctx, "api-key-abcdef123456")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.InfoContext(ctx, "request processed",
			"trace_id", "abc-123",
			"status", 200,
			"some_field", "Bearer super-secret-token-12345",
		)
	}
}

// BenchmarkRedactingHandler_HTTPHeader benchmarks redaction of http.Header values.
// Target: < 3us
func BenchmarkRedactingHandler_HTTPHeader(b *testing.B) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	sensitiveHeaders := []string{"Authorization", "Cookie", "X-API-Key"}
	handler := NewRedactingHandler(inner, sensitiveHeaders, false)
	logger := slog.New(handler)
	ctx := context.Background()

	headers := http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {"Bearer secret-token"},
		"X-Request-ID":  {"trace-123"},
		"Cookie":        {"session=abc123"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.InfoContext(ctx, "request headers",
			"headers", headers,
		)
	}
}

// BenchmarkRedactingHandler_SensitiveKey benchmarks redaction of sensitive key names.
// Target: < 2us
func BenchmarkRedactingHandler_SensitiveKey(b *testing.B) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	sensitiveHeaders := []string{"Authorization", "Cookie", "X-API-Key"}
	handler := NewRedactingHandler(inner, sensitiveHeaders, false)
	logger := slog.New(handler)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.InfoContext(ctx, "auth info",
			"authorization", "Bearer secret-token",
			"cookie", "session=abc",
		)
	}
}

// BenchmarkRedactingHandler_Parallel tests concurrent log redaction.
// Uses io.Discard instead of shared bytes.Buffer to avoid data race.
func BenchmarkRedactingHandler_Parallel(b *testing.B) {
	inner := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	sensitiveHeaders := []string{"Authorization", "Cookie", "X-API-Key"}
	handler := NewRedactingHandler(inner, sensitiveHeaders, false)
	logger := slog.New(handler)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		ctx = WithSecretValue(ctx, "Bearer token-12345678")

		for pb.Next() {
			logger.InfoContext(ctx, "request processed",
				"trace_id", "abc-123",
				"status", 200,
			)
		}
	})
}
