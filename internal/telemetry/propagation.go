// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"hash/fnv"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

// BridgeConnectRequestID converts a Connect-Request-ID to an OTel trace ID.
// Accepts 32-char hex strings, UUIDs (dashes stripped), or arbitrary strings (hashed).
func BridgeConnectRequestID(connectRequestID string) (trace.TraceID, bool) {
	if connectRequestID == "" {
		return trace.TraceID{}, false
	}

	// Strip dashes (handles both 32-char hex and UUID formats).
	// TraceIDFromHex validates hex characters, so no manual format checks needed.
	cleaned := strings.ReplaceAll(connectRequestID, "-", "")
	if len(cleaned) == 32 {
		if id, err := trace.TraceIDFromHex(cleaned); err == nil {
			return id, true
		}
	}

	// Fallback: Hash to create deterministic trace ID
	return hashToTraceID(connectRequestID), true
}

// hashToTraceID uses FNV-128a from the standard library to produce a
// full 128-bit trace ID from an arbitrary string.
func hashToTraceID(s string) trace.TraceID {
	h := fnv.New128a()
	_, _ = h.Write([]byte(s))
	var id trace.TraceID
	copy(id[:], h.Sum(nil))
	return id
}

// hashToSpanID uses FNV-64a to derive a deterministic span ID from a string.
func hashToSpanID(s string) trace.SpanID {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	var id trace.SpanID
	copy(id[:], h.Sum(nil))
	return id
}

// upstreamSpanKey stores the upstream span in the request context so that
// ModifyResponse and ErrorHandler can retrieve and end it.
type upstreamSpanKeyType struct{}

// WithUpstreamSpan stores the upstream span in the context.
// The span must be ended by the caller via EndUpstreamSpan.
func WithUpstreamSpan(ctx context.Context, span trace.Span) context.Context {
	return context.WithValue(ctx, upstreamSpanKeyType{}, span)
}

// UpstreamSpanFromContext retrieves the upstream span from context.
// Returns nil if not present.
func UpstreamSpanFromContext(ctx context.Context) trace.Span {
	span, _ := ctx.Value(upstreamSpanKeyType{}).(trace.Span)
	return span
}

// EndUpstreamSpan retrieves the upstream span from context, records the
// response status code, and ends it. Safe to call when span is nil.
func EndUpstreamSpan(ctx context.Context, statusCode int, err error) {
	span := UpstreamSpanFromContext(ctx)
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetAttributes(semconv.HTTPResponseStatusCode(statusCode))
		if statusCode >= 500 {
			span.SetStatus(codes.Error, http.StatusText(statusCode))
		}
	}
	span.End()
}

// InjectTraceContext injects W3C Trace Context headers into the request.
func InjectTraceContext(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

// ExtractTraceContext extracts W3C Trace Context from incoming request headers.
func ExtractTraceContext(ctx context.Context, req *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(req.Header))
}

// SpanContextFromConnectID creates a SpanContext using the bridged trace ID.
func SpanContextFromConnectID(connectRequestID string) (trace.SpanContext, bool) {
	traceID, ok := BridgeConnectRequestID(connectRequestID)
	if !ok {
		return trace.SpanContext{}, false
	}

	// Derive SpanID deterministically from the request ID so each
	// bridged request gets a unique, reproducible SpanID.
	spanID := hashToSpanID(connectRequestID)

	cfg := trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}
	sc := trace.NewSpanContext(cfg)
	if !sc.IsValid() {
		return trace.SpanContext{}, false
	}
	return sc, true
}
