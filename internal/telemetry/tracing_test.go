// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setupTestTracer configures a test tracer provider with an in-memory span recorder.
// NOTE: Tests using this must NOT use t.Parallel() because they share
// global OTel state (TracerProvider and TextMapPropagator).
func setupTestTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))

	oldTP := otel.GetTracerProvider()
	oldProp := otel.GetTextMapPropagator()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	t.Cleanup(func() {
		otel.SetTracerProvider(oldTP)
		otel.SetTextMapPropagator(oldProp)
		_ = tp.Shutdown(context.Background())
	})

	return recorder
}

func TestBridgeConnectRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantOK  bool
		wantHex string
	}{
		{
			name:   "empty string",
			input:  "",
			wantOK: false,
		},
		{
			name:    "valid UUID",
			input:   "550e8400-e29b-41d4-a716-446655440000",
			wantOK:  true,
			wantHex: "550e8400e29b41d4a716446655440000",
		},
		{
			name:    "32-char hex string",
			input:   "550e8400e29b41d4a716446655440000",
			wantOK:  true,
			wantHex: "550e8400e29b41d4a716446655440000",
		},
		{
			name:   "arbitrary string (hashed)",
			input:  "request-12345",
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			traceID, ok := BridgeConnectRequestID(tt.input)

			if ok != tt.wantOK {
				t.Errorf("BridgeConnectRequestID(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}

			if tt.wantHex != "" {
				got := traceID.String()
				if got != tt.wantHex {
					t.Errorf("BridgeConnectRequestID(%q) = %s, want %s", tt.input, got, tt.wantHex)
				}
			}

			// Verify determinism
			if ok && tt.wantHex == "" {
				traceID2, _ := BridgeConnectRequestID(tt.input)
				if traceID != traceID2 {
					t.Errorf("BridgeConnectRequestID is not deterministic")
				}
			}
		})
	}
}

func TestBridgeConnectRequestID_DifferentInputsProduceDifferentIDs(t *testing.T) {
	t.Parallel()

	id1, ok1 := BridgeConnectRequestID("request-aaa")
	id2, ok2 := BridgeConnectRequestID("request-bbb")

	if !ok1 || !ok2 {
		t.Fatal("expected both to return ok=true")
	}
	if id1 == id2 {
		t.Error("different inputs should produce different trace IDs")
	}
}

func TestSpanContextFromConnectID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantOK    bool
		wantValid bool
	}{
		{
			name:   "empty string returns invalid",
			input:  "",
			wantOK: false,
		},
		{
			name:      "valid UUID produces valid SpanContext",
			input:     "550e8400-e29b-41d4-a716-446655440000",
			wantOK:    true,
			wantValid: true,
		},
		{
			name:      "arbitrary string produces valid SpanContext",
			input:     "my-request-id",
			wantOK:    true,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sc, ok := SpanContextFromConnectID(tt.input)
			if ok != tt.wantOK {
				t.Errorf("SpanContextFromConnectID(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if tt.wantValid && !sc.IsValid() {
				t.Error("expected valid SpanContext")
			}
			if tt.wantValid && !sc.IsRemote() {
				t.Error("expected remote SpanContext")
			}
		})
	}

	// Verify different inputs produce different SpanIDs
	sc1, _ := SpanContextFromConnectID("request-1")
	sc2, _ := SpanContextFromConnectID("request-2")
	if sc1.SpanID() == sc2.SpanID() {
		t.Error("different inputs should produce different SpanIDs")
	}
}

func TestTracingMiddleware_CreatesSpan(t *testing.T) {
	recorder := setupTestTracer(t)

	handler := TracingMiddleware(
		otel.Tracer(TracerName),
		"X-Connect",
		"Connect-Request-ID",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name() != "GET /proxy" {
		t.Errorf("span name = %q, want %q", span.Name(), "GET /proxy")
	}

	// Verify vendor_id attribute
	foundVendor := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == "chaperone.vendor_id" {
			foundVendor = true
			if attr.Value.AsString() != "test-vendor" {
				t.Errorf("vendor_id = %q, want %q", attr.Value.AsString(), "test-vendor")
			}
		}
	}
	if !foundVendor {
		t.Error("vendor_id attribute not found")
	}
}

func TestTracingMiddleware_W3CTraceparentTakesPriorityOverConnectRequestID(t *testing.T) {
	recorder := setupTestTracer(t)

	handler := TracingMiddleware(
		otel.Tracer(TracerName),
		"X-Connect",
		"Connect-Request-ID",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	// Create a valid W3C traceparent with a known trace ID.
	w3cTraceID := "abcdef0123456789abcdef0123456789"
	traceparent := "00-" + w3cTraceID + "-00f067aa0ba902b7-01"

	// Also set a Connect-Request-ID that would produce a different trace ID.
	connectRequestID := "550e8400-e29b-41d4-a716-446655440000"

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("traceparent", traceparent)
	req.Header.Set("Connect-Request-ID", connectRequestID)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	gotTraceID := span.SpanContext().TraceID().String()
	if gotTraceID != w3cTraceID {
		t.Errorf("span trace ID = %q, want W3C trace ID %q (not bridged Connect-Request-ID)", gotTraceID, w3cTraceID)
	}
}

func TestTracingMiddleware_DoesNotLeakQueryParams(t *testing.T) {
	recorder := setupTestTracer(t)

	handler := TracingMiddleware(
		otel.Tracer(TracerName),
		"X-Connect",
		"Connect-Request-ID",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/proxy?api_key=SECRET_TOKEN", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Verify no span attribute contains the secret query parameter
	for _, attr := range spans[0].Attributes() {
		if strings.Contains(attr.Value.AsString(), "SECRET_TOKEN") {
			t.Errorf("span attribute %q leaks query parameter: %q", attr.Key, attr.Value.AsString())
		}
		if strings.Contains(attr.Value.AsString(), "api_key") {
			t.Errorf("span attribute %q leaks query parameter key: %q", attr.Key, attr.Value.AsString())
		}
	}
}

func TestTracingMiddleware_5xxSetsErrorStatus(t *testing.T) {
	recorder := setupTestTracer(t)

	handler := TracingMiddleware(
		otel.Tracer(TracerName),
		"X-Connect",
		"Connect-Request-ID",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Status().Code != otelcodes.Error {
		t.Errorf("span status code = %v, want Error for 500", span.Status().Code)
	}
}

func TestTracingMiddleware_4xxLeavesStatusUnset(t *testing.T) {
	recorder := setupTestTracer(t)

	handler := TracingMiddleware(
		otel.Tracer(TracerName),
		"X-Connect",
		"Connect-Request-ID",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	// Per OTel semantic conventions, 4xx on server spans must NOT be Error
	if span.Status().Code == otelcodes.Error {
		t.Errorf("span status should NOT be Error for 4xx (got %v)", span.Status().Code)
	}
}

func TestStartPluginSpan_CreatesChildSpan(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, parentSpan := otel.Tracer(TracerName).Start(context.Background(), "parent")
	defer parentSpan.End()

	_, pluginSpan := StartPluginSpan(ctx, "GetCredentials", "vendor-123")
	pluginSpan.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name() != "plugin.GetCredentials" {
		t.Errorf("span name = %q, want %q", span.Name(), "plugin.GetCredentials")
	}

	if span.Parent().SpanID() != parentSpan.SpanContext().SpanID() {
		t.Error("plugin span should be child of parent span")
	}
}

func TestStartUpstreamSpan_InjectsHeaders(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, parentSpan := otel.Tracer(TracerName).Start(context.Background(), "parent")
	defer parentSpan.End()

	req := httptest.NewRequest(http.MethodGet, "https://api.vendor.com/v1/test", nil)
	_, upstreamSpan := StartUpstreamSpan(ctx, req, "api.vendor.com")
	upstreamSpan.End()

	// Verify W3C traceparent header is injected
	traceparent := req.Header.Get("traceparent")
	if traceparent == "" {
		t.Error("traceparent header not injected")
	}
	// Verify W3C traceparent format: version-traceid-spanid-flags
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		t.Errorf("traceparent format invalid, expected 4 parts, got %d: %q", len(parts), traceparent)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name() != "GET api.vendor.com" {
		t.Errorf("span name = %q, want %q", span.Name(), "GET api.vendor.com")
	}
}

func TestStartUpstreamSpan_DoesNotLeakQueryParams(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, parentSpan := otel.Tracer(TracerName).Start(context.Background(), "parent")
	defer parentSpan.End()

	req := httptest.NewRequest(http.MethodGet, "https://api.vendor.com/v1/test?token=SECRET", nil)
	_, upstreamSpan := StartUpstreamSpan(ctx, req, "api.vendor.com")
	upstreamSpan.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	// Verify no span attribute contains the secret query parameter
	for _, attr := range spans[0].Attributes() {
		if strings.Contains(attr.Value.AsString(), "SECRET") {
			t.Errorf("upstream span attribute %q leaks query parameter: %q", attr.Key, attr.Value.AsString())
		}
		if strings.Contains(attr.Value.AsString(), "token=") {
			t.Errorf("upstream span attribute %q leaks query key: %q", attr.Key, attr.Value.AsString())
		}
	}
}

func TestRecordSpanError(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, span := otel.Tracer(TracerName).Start(context.Background(), "test-op")
	testErr := errors.New("connection refused")
	RecordSpanError(ctx, testErr)
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Status().Code != otelcodes.Error {
		t.Errorf("span status = %v, want Error", s.Status().Code)
	}
	if s.Status().Description != "connection refused" {
		t.Errorf("span status description = %q, want %q", s.Status().Description, "connection refused")
	}

	// Verify the error event was recorded
	foundErr := false
	for _, event := range s.Events() {
		if event.Name == "exception" {
			foundErr = true
		}
	}
	if !foundErr {
		t.Error("expected exception event to be recorded")
	}
}

func TestInitTracing_Disabled(t *testing.T) {
	shutdown, err := InitTracing(context.Background(), TracingConfig{
		ServiceName:    "test",
		ServiceVersion: "0.0.1",
		Enabled:        false,
	})
	if err != nil {
		t.Fatalf("InitTracing(disabled) returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function even when disabled")
	}
	// Shutdown should be a no-op and not error
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown(disabled) returned error: %v", err)
	}
}

func TestIsTracingEnabled(t *testing.T) {
	tests := []struct {
		envValue string
		want     bool
	}{
		{"", true},
		{"false", true},
		{"0", true},
		{"true", false},
		{"True", false},
		{"TRUE", false},
	}

	for _, tt := range tests {
		t.Run("OTEL_SDK_DISABLED="+tt.envValue, func(t *testing.T) {
			t.Setenv("OTEL_SDK_DISABLED", tt.envValue)
			if got := IsTracingEnabled(); got != tt.want {
				t.Errorf("IsTracingEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpstreamSpan_EndWithError(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, parentSpan := otel.Tracer(TracerName).Start(context.Background(), "parent")
	defer parentSpan.End()

	req := httptest.NewRequest(http.MethodGet, "https://api.vendor.com/v1/test", nil)
	ctx, upstreamSpan := StartUpstreamSpan(ctx, req, "api.vendor.com")
	ctx = WithUpstreamSpan(ctx, upstreamSpan)

	// Simulate a transport error
	testErr := errors.New("connection reset by peer")
	EndUpstreamSpan(ctx, 0, testErr)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	span := spans[0]
	if span.Status().Code != otelcodes.Error {
		t.Errorf("span status = %v, want Error", span.Status().Code)
	}
	if span.Status().Description != "connection reset by peer" {
		t.Errorf("span status description = %q, want %q", span.Status().Description, "connection reset by peer")
	}
}

func TestUpstreamSpan_EndWithSuccessStatus(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, parentSpan := otel.Tracer(TracerName).Start(context.Background(), "parent")
	defer parentSpan.End()

	req := httptest.NewRequest(http.MethodGet, "https://api.vendor.com/v1/test", nil)
	ctx, upstreamSpan := StartUpstreamSpan(ctx, req, "api.vendor.com")
	ctx = WithUpstreamSpan(ctx, upstreamSpan)

	// Simulate a successful response
	EndUpstreamSpan(ctx, 200, nil)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	span := spans[0]
	// Success should NOT set error status
	if span.Status().Code == otelcodes.Error {
		t.Error("span status should NOT be Error for 2xx")
	}
}

func TestUpstreamSpan_EndWith5xxSetsError(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, parentSpan := otel.Tracer(TracerName).Start(context.Background(), "parent")
	defer parentSpan.End()

	req := httptest.NewRequest(http.MethodGet, "https://api.vendor.com/v1/test", nil)
	ctx, upstreamSpan := StartUpstreamSpan(ctx, req, "api.vendor.com")
	ctx = WithUpstreamSpan(ctx, upstreamSpan)

	// Simulate a 5xx response
	EndUpstreamSpan(ctx, 503, nil)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	span := spans[0]
	// 5xx should set error status per OTel semantic conventions
	if span.Status().Code != otelcodes.Error {
		t.Error("span status should be Error for 5xx")
	}
}

func TestUpstreamSpan_NilSpanIsNoOp(t *testing.T) {
	// EndUpstreamSpan should be safe to call with nil span
	ctx := context.Background()
	// This should not panic
	EndUpstreamSpan(ctx, 200, nil)
	EndUpstreamSpan(ctx, 0, errors.New("test error"))
}
