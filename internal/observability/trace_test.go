// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- TraceIDFromContext / WithTraceID Tests ---

func TestTraceIDFromContext_NoValue_ReturnsEmpty(t *testing.T) {
	ctx := context.Background()

	got := TraceIDFromContext(ctx)

	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestWithTraceID_StoresAndRetrieves(t *testing.T) {
	ctx := context.Background()
	expected := "abc-123-def-456"

	ctx = WithTraceID(ctx, expected)
	got := TraceIDFromContext(ctx)

	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestWithTraceID_OverwritesPrevious(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "first")
	ctx = WithTraceID(ctx, "second")

	got := TraceIDFromContext(ctx)
	if got != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

// --- ExtractOrGenerateTraceID Tests ---

func TestExtractOrGenerateTraceID_HeaderPresent_ReturnsHeaderValue(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r.Header.Set("Connect-Request-ID", "upstream-trace-abc")

	got := ExtractOrGenerateTraceID(r, "Connect-Request-ID")

	if got != "upstream-trace-abc" {
		t.Errorf("got %q, want %q", got, "upstream-trace-abc")
	}
}

func TestExtractOrGenerateTraceID_HeaderMissing_GeneratesUUID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)

	got := ExtractOrGenerateTraceID(r, "Connect-Request-ID")

	if got == "" {
		t.Fatal("expected generated trace ID, got empty string")
	}
	// UUIDv4 format: 8-4-4-4-12 hex chars
	if len(got) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(got), got)
	}
}

func TestExtractOrGenerateTraceID_EmptyHeaderValue_GeneratesUUID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r.Header.Set("Connect-Request-ID", "")

	got := ExtractOrGenerateTraceID(r, "Connect-Request-ID")

	if got == "" {
		t.Fatal("expected generated trace ID, got empty string")
	}
	if len(got) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(got), got)
	}
}

func TestExtractOrGenerateTraceID_CustomHeaderName(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r.Header.Set("X-Custom-Trace", "custom-trace-id")

	got := ExtractOrGenerateTraceID(r, "X-Custom-Trace")

	if got != "custom-trace-id" {
		t.Errorf("got %q, want %q", got, "custom-trace-id")
	}
}

func TestExtractOrGenerateTraceID_GeneratedIDsAreUnique(t *testing.T) {
	r1 := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r2 := httptest.NewRequest(http.MethodGet, "/proxy", nil)

	id1 := ExtractOrGenerateTraceID(r1, "Connect-Request-ID")
	id2 := ExtractOrGenerateTraceID(r2, "Connect-Request-ID")

	if id1 == id2 {
		t.Errorf("expected unique IDs, got same: %q", id1)
	}
}

// --- Trace ID Validation Tests (defense-in-depth) ---

func TestExtractOrGenerateTraceID_TooLong_GeneratesNew(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	// 257 chars exceeds maxTraceIDLen (256)
	r.Header.Set("Connect-Request-ID", strings.Repeat("a", maxTraceIDLen+1))

	got := ExtractOrGenerateTraceID(r, "Connect-Request-ID")

	// Should get a generated UUID, not the oversized value
	if len(got) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(got), got)
	}
}

func TestExtractOrGenerateTraceID_MaxLength_Accepted(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	maxID := strings.Repeat("a", maxTraceIDLen)
	r.Header.Set("Connect-Request-ID", maxID)

	got := ExtractOrGenerateTraceID(r, "Connect-Request-ID")

	if got != maxID {
		t.Errorf("expected trace ID to be accepted at max length, got len=%d", len(got))
	}
}

func TestExtractOrGenerateTraceID_InvalidChars_GeneratesNew(t *testing.T) {
	tests := []struct {
		name    string
		traceID string
	}{
		{"null byte", "trace\x00id"},
		{"tab character", "trace\tid"},
		{"newline", "trace\nid"},
		{"carriage return", "trace\rid"},
		{"angle brackets", "trace<script>id"},
		{"curly braces", "trace{inject}id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			r.Header.Set("Connect-Request-ID", tt.traceID)

			got := ExtractOrGenerateTraceID(r, "Connect-Request-ID")

			if got == tt.traceID {
				t.Errorf("expected rejection of %q, but it was accepted", tt.traceID)
			}
			if len(got) != 36 {
				t.Errorf("expected UUID length 36, got %d: %q", len(got), got)
			}
		})
	}
}

func TestExtractOrGenerateTraceID_ValidFormats_Accepted(t *testing.T) {
	tests := []struct {
		name    string
		traceID string
	}{
		{"UUID format", "550e8400-e29b-41d4-a716-446655440000"},
		{"W3C traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{"alphanumeric", "abc123DEF456"},
		{"with underscores", "trace_id_123"},
		{"with dots", "req.trace.123"},
		{"with colons", "span:trace:123"},
		{"base64-like", "dHJhY2UtaWQ=/+abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			r.Header.Set("Connect-Request-ID", tt.traceID)

			got := ExtractOrGenerateTraceID(r, "Connect-Request-ID")

			if got != tt.traceID {
				t.Errorf("got %q, want %q", got, tt.traceID)
			}
		})
	}
}

// --- TraceIDMiddleware Tests ---

func TestTraceIDMiddleware_ExtractsFromHeader(t *testing.T) {
	var capturedTraceID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = TraceIDFromContext(r.Context())
	})

	handler := TraceIDMiddleware("Connect-Request-ID", inner)
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r.Header.Set("Connect-Request-ID", "from-upstream-abc")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if capturedTraceID != "from-upstream-abc" {
		t.Errorf("got %q, want %q", capturedTraceID, "from-upstream-abc")
	}
}

func TestTraceIDMiddleware_GeneratesWhenMissing(t *testing.T) {
	var capturedTraceID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = TraceIDFromContext(r.Context())
	})

	handler := TraceIDMiddleware("Connect-Request-ID", inner)
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if capturedTraceID == "" {
		t.Fatal("expected generated trace ID, got empty string")
	}
	if len(capturedTraceID) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(capturedTraceID), capturedTraceID)
	}
}

func TestTraceIDMiddleware_SetsHeaderOnRequest(t *testing.T) {
	var capturedHeader string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("Connect-Request-ID")
	})

	handler := TraceIDMiddleware("Connect-Request-ID", inner)
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	// No trace header set — middleware should generate and set it
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if capturedHeader == "" {
		t.Fatal("expected trace ID header to be set on request")
	}
	// Should match what's in context
	if len(capturedHeader) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(capturedHeader), capturedHeader)
	}
}

func TestTraceIDMiddleware_PropagatesExistingHeader(t *testing.T) {
	var capturedHeader string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("Connect-Request-ID")
	})

	handler := TraceIDMiddleware("Connect-Request-ID", inner)
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r.Header.Set("Connect-Request-ID", "upstream-id-xyz")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if capturedHeader != "upstream-id-xyz" {
		t.Errorf("got %q, want %q", capturedHeader, "upstream-id-xyz")
	}
}
