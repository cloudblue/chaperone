// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

// TracerName is the instrumentation scope name for Chaperone spans.
const TracerName = "github.com/cloudblue/chaperone"

// EnvOTelSDKDisabled is the standard OTel env var to disable the SDK.
const EnvOTelSDKDisabled = "OTEL_SDK_DISABLED"

// TracingConfig holds configuration for OpenTelemetry tracing.
type TracingConfig struct {
	ServiceName    string
	ServiceVersion string
	Enabled        bool
}

// IsTracingEnabled checks if tracing should be enabled based on env vars.
// Per OTel specification, OTEL_SDK_DISABLED is compared case-insensitively.
func IsTracingEnabled() bool {
	return !strings.EqualFold(os.Getenv(EnvOTelSDKDisabled), "true")
}

// InitTracing initializes the OpenTelemetry TracerProvider with OTLP exporter.
func InitTracing(ctx context.Context, cfg TracingConfig) (shutdown func(context.Context) error, err error) {
	if !cfg.Enabled {
		slog.Info("tracing disabled via configuration")
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
		// WithFromEnv reads OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES.
		// It appears after WithAttributes so env values override code defaults.
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		// SECURITY: Do NOT use resource.WithProcess() — it includes
		// WithProcessCommandArgs which would export CLI flags like
		// -credentials <path> to the tracing backend.
		resource.WithProcessPID(),
		resource.WithProcessExecutableName(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		// Sampler defaults to ParentBased(AlwaysSample).
		// Override via OTEL_TRACES_SAMPLER / OTEL_TRACES_SAMPLER_ARG env vars.
		// NOTE: Do NOT add WithSampler() here — it overrides env var config,
		// preventing operators from tuning sampling in production.
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("tracing initialized",
		"service_name", cfg.ServiceName,
		"service_version", cfg.ServiceVersion,
	)

	return func(ctx context.Context) error {
		slog.Info("shutting down tracer provider")
		if err := tp.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutting down tracer provider: %w", err)
		}
		return nil
	}, nil
}

// Tracer returns a named tracer for creating spans.
func Tracer() trace.Tracer {
	return otel.Tracer(TracerName)
}
