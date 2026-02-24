// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/cloudblue/chaperone/internal/httputil"
	"github.com/cloudblue/chaperone/internal/observability"
)

// Span attribute keys for Chaperone-specific span attributes.
const (
	AttrVendorID         = attribute.Key("chaperone.vendor_id")
	AttrServiceID        = attribute.Key("chaperone.service_id")
	AttrConnectRequestID = attribute.Key("chaperone.connect_request_id")
)

// TracingMiddleware instruments HTTP handlers with OpenTelemetry tracing.
//
// NOTE: This middleware must run AFTER TraceIDMiddleware in the middleware chain
// (i.e., TraceIDMiddleware is outermost) so that TraceIDFromContext returns
// the Connect-Request-ID. If ordering changes, the fallback reads the header directly.
//
//nolint:gocognit // Sequential instrumentation steps are inherently linear; splitting would obscure the flow.
func TracingMiddleware(tracer trace.Tracer, headerPrefix, traceHeader string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1. Try W3C Trace Context extraction first (standard propagation).
		ctx = ExtractTraceContext(ctx, r)

		// 2. If no valid trace context from W3C headers, bridge from Connect-Request-ID.
		// Primary: read from context (set by TraceIDMiddleware).
		// Fallback: read header directly if middleware ordering changes.
		connectRequestID := observability.TraceIDFromContext(ctx)
		if connectRequestID == "" {
			connectRequestID = r.Header.Get(traceHeader)
		}

		if !trace.SpanContextFromContext(ctx).IsValid() && connectRequestID != "" {
			if sc, ok := SpanContextFromConnectID(connectRequestID); ok {
				ctx = trace.ContextWithSpanContext(ctx, sc)
			}
		}

		// Span name follows OTel HTTP semantic conventions: "METHOD /path".
		spanName := r.Method + " " + r.URL.Path

		// SECURITY: Record only path (not query string) to prevent leaking
		// API keys or tokens that may appear in query parameters.
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.URLPath(r.URL.Path),
				semconv.URLScheme(scheme),
				semconv.ServerAddress(r.Host),
			),
		)
		defer span.End()

		// Normalize vendor ID to prevent unbounded cardinality from
		// malicious or misconfigured clients (matches MetricsMiddleware behavior).
		vendorID := NormalizeVendorID(r.Header.Get(headerPrefix + "-Vendor-ID"))
		span.SetAttributes(AttrVendorID.String(vendorID))

		// Normalize service ID with same validation as vendor ID to prevent
		// unbounded cardinality from malicious or misconfigured clients.
		serviceID := NormalizeVendorID(r.Header.Get(headerPrefix + "-Service-ID"))
		if serviceID != DefaultVendorID {
			span.SetAttributes(AttrServiceID.String(serviceID))
		}

		if connectRequestID != "" {
			span.SetAttributes(AttrConnectRequestID.String(connectRequestID))
		}

		// Reuse existing StatusCapturingResponseWriter if available
		// (shared with MetricsMiddleware — both read Status after handler completes).
		wrapped, ok := w.(*httputil.StatusCapturingResponseWriter)
		if !ok {
			wrapped = httputil.NewStatusCapturingResponseWriter(w)
		}
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		span.SetAttributes(semconv.HTTPResponseStatusCode(wrapped.Status))

		// Per OTel HTTP semantic conventions, only 5xx responses are errors
		// on server spans. 4xx responses are correctly handled client errors
		// and must NOT set span status or error.type.
		// Ref: https://opentelemetry.io/docs/specs/semconv/http/http-spans/#status
		if wrapped.Status >= 500 {
			span.SetAttributes(semconv.ErrorTypeKey.String(strconv.Itoa(wrapped.Status)))
			span.SetStatus(codes.Error, http.StatusText(wrapped.Status))
		}
	})
}

// StartPluginSpan creates a span for plugin execution.
// When tracing is disabled (OTEL_SDK_DISABLED=true), the global TracerProvider
// returns no-op spans, so this is safe to call unconditionally. The overhead
// of no-op span creation is negligible compared to plugin execution.
func StartPluginSpan(ctx context.Context, operation, vendorID string) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, "plugin."+operation,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(AttrVendorID.String(vendorID)),
	)
	return ctx, span
}

// StartUpstreamSpan creates a span for upstream HTTP calls.
// SECURITY: Records only host+path (no query string) to prevent leaking
// API keys or tokens that may appear in query parameters.
// SECURITY: url.full is intentionally omitted — Chaperone injects credentials
// into URLs/headers, so the full URL could contain secrets.
// When tracing is disabled (OTEL_SDK_DISABLED=true), the global TracerProvider
// returns no-op spans, so this is safe to call unconditionally. The overhead
// of no-op span creation and header injection is negligible.
func StartUpstreamSpan(ctx context.Context, req *http.Request, targetHost string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLPath(req.URL.Path),
		semconv.ServerAddress(targetHost),
	}
	if port := req.URL.Port(); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			attrs = append(attrs, semconv.ServerPort(p))
		}
	}

	ctx, span := Tracer().Start(ctx, req.Method+" "+targetHost,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)

	InjectTraceContext(ctx, req)
	return ctx, span
}

// RecordSpanError records an error on the current span.
func RecordSpanError(ctx context.Context, err error) {
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
