# Task: Telemetry (OpenTelemetry Tracing)

**Status:** [ ] Not Started
**Priority:** P1
**Estimated Effort:** L

## Objective

Implement OpenTelemetry integration with OTLP exporters for distributed tracing.

## Design Spec Reference

- **Primary:** Section 8.3.1 - Distributed Tracing
- **Primary:** Section 8.3 Note on OpenTelemetry
- **Related:** Section 5.2.B - Trace ID propagation

## Dependencies

- [ ] `07-telemetry-metrics.task.md` - Shares admin infrastructure
- [x] Phase 1 complete - Trace ID extraction exists
- [ ] **NO dependencies on Tasks 01-06** (enables parallel development)

## Acceptance Criteria

- [ ] OpenTelemetry SDK integrated with OTLP exporter
- [ ] Traces exported to configurable OTLP endpoint (via `OTEL_EXPORTER_OTLP_ENDPOINT`)
- [ ] Standard OTel environment variables respected (`OTEL_*`)
- [ ] Spans created for: request handling, plugin execution, upstream call
- [ ] Existing `Connect-Request-ID` used as trace ID when present
- [ ] W3C Trace Context headers propagated to upstream
- [ ] Span attributes include: `vendor_id`, `service_id`, `status_code`
- [ ] Tracing can be disabled via environment variable
- [ ] Tests pass: `go test ./internal/telemetry/...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Add OpenTelemetry dependencies:
   - `go.opentelemetry.io/otel`
   - `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
   - `go.opentelemetry.io/otel/sdk/trace`
2. Create `internal/telemetry/tracing.go` for OTel setup
3. Create tracing middleware that wraps requests
4. Integrate with existing trace ID extraction
5. Add span events for key lifecycle points

### OTel Initialization

```go
package telemetry

import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func InitTracing(ctx context.Context, serviceName, version string) (*sdktrace.TracerProvider, error) {
    // OTLP exporter uses OTEL_EXPORTER_OTLP_ENDPOINT env var by default
    exporter, err := otlptracehttp.New(ctx)
    if err != nil {
        return nil, fmt.Errorf("creating OTLP exporter: %w", err)
    }

    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceName(serviceName),
            semconv.ServiceVersion(version),
        ),
    )
    if err != nil {
        return nil, fmt.Errorf("creating resource: %w", err)
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(res),
    )
    otel.SetTracerProvider(tp)
    
    return tp, nil
}
```

### Tracing Middleware

```go
func TracingMiddleware(tracer trace.Tracer, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract existing trace ID or create new span
        ctx, span := tracer.Start(r.Context(), "handle_request",
            trace.WithSpanKind(trace.SpanKindServer),
        )
        defer span.End()

        // Add attributes
        vendorID := r.Header.Get("X-Connect-Vendor-ID")
        span.SetAttributes(
            attribute.String("vendor_id", vendorID),
            attribute.String("http.method", r.Method),
            attribute.String("http.url", r.URL.Path),
        )

        wrapped := &responseWrapper{ResponseWriter: w, status: 200}
        next.ServeHTTP(wrapped, r.WithContext(ctx))

        span.SetAttributes(
            attribute.Int("http.status_code", wrapped.status),
        )
        if wrapped.status >= 400 {
            span.SetStatus(codes.Error, "request failed")
        }
    })
}
```

### Upstream Call Tracing

```go
func TraceUpstreamCall(ctx context.Context, tracer trace.Tracer, req *http.Request) (*http.Response, error) {
    ctx, span := tracer.Start(ctx, "upstream_call",
        trace.WithSpanKind(trace.SpanKindClient),
    )
    defer span.End()

    span.SetAttributes(
        attribute.String("http.url", req.URL.String()),
        attribute.String("http.method", req.Method),
    )

    // Propagate context to upstream
    otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

    resp, err := http.DefaultClient.Do(req.WithContext(ctx))
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return nil, err
    }

    span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
    return resp, nil
}
```

### Environment Variables (Standard OTel)

```bash
# Enable/disable
OTEL_SDK_DISABLED=false

# Exporter endpoint
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# Service identification
OTEL_SERVICE_NAME=chaperone
```

### Key Code Locations

- `internal/telemetry/tracing.go` - OTel initialization
- `internal/telemetry/tracing_middleware.go` - Request tracing
- `internal/telemetry/propagation.go` - Context propagation
- `cmd/chaperone/main.go` - Initialize tracing

### Gotchas

- Shutdown: TracerProvider must be shut down gracefully to flush spans
- Sampling: Consider sampler configuration for high-volume production
- Context propagation: Ensure request context flows through all layers
- Trace ID bridging: Connect's `Connect-Request-ID` may need to be bridged to W3C format
- Dependencies: OTel adds several transitive dependencies; verify compatibility

## Files to Create/Modify

- [ ] `internal/telemetry/tracing.go` - OTel setup
- [ ] `internal/telemetry/tracing_middleware.go` - Middleware
- [ ] `internal/telemetry/propagation.go` - Context propagation
- [ ] `internal/telemetry/tracing_test.go` - Unit tests
- [ ] `cmd/chaperone/main.go` - Initialize and shutdown tracing
- [ ] `go.mod` - Add OTel dependencies

## Testing Strategy

- **Unit tests:**
  - Span creation and attributes
  - Context propagation
  - Error recording
- **Integration tests:**
  - Traces exported to mock collector
  - Parent-child span relationships
  - Trace ID continuity
- **Manual verification:**
  - View traces in Jaeger/Zipkin
  - Verify span hierarchy
