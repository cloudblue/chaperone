// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package chaperone provides the public API for the Chaperone egress proxy.
//
// Distributors building custom binaries with their own plugins should import
// this package and call [Run] as their entry point. All internal implementation
// details (config loading, TLS, middleware, etc.) are handled automatically.
//
// Example (Distributor's main.go):
//
//	package main
//
//	import (
//	    "context"
//	    "os/signal"
//	    "syscall"
//
//	    "github.com/cloudblue/chaperone"
//	    "github.com/acme/my-proxy/plugins"
//	)
//
//	func main() {
//	    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
//	    defer stop()
//
//	    if err := chaperone.Run(ctx, plugins.New()); err != nil {
//	        os.Exit(1)
//	    }
//	}
package chaperone

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/internal/telemetry"
	"github.com/cloudblue/chaperone/sdk"
)

// runConfig holds the resolved options for a Run call.
type runConfig struct {
	configPath string
	version    string
	commit     string
	buildDate  string
	logOutput  io.Writer
}

// Option configures optional behavior for [Run].
type Option func(*runConfig)

// WithConfigPath sets the path to the YAML configuration file.
//
// If not set, Chaperone resolves the config path in this order:
//  1. CHAPERONE_CONFIG environment variable
//  2. ./config.yaml (current directory)
func WithConfigPath(path string) Option {
	return func(c *runConfig) {
		c.configPath = path
	}
}

// WithVersion sets the version string reported by the /_ops/version endpoint
// and included in startup logs.
func WithVersion(version string) Option {
	return func(c *runConfig) {
		c.version = version
	}
}

// WithBuildInfo sets the git commit and build date metadata included in
// startup logs. If not set, these fields are omitted.
func WithBuildInfo(commit, buildDate string) Option {
	return func(c *runConfig) {
		c.commit = commit
		c.buildDate = buildDate
	}
}

// WithLogOutput sets the [io.Writer] for structured log output.
// Defaults to [os.Stdout]. Override for testing or custom log routing.
func WithLogOutput(w io.Writer) Option {
	return func(c *runConfig) {
		c.logOutput = w
	}
}

// Run starts the Chaperone proxy with the given plugin and blocks until
// the context is cancelled or a fatal error occurs.
//
// This is the primary entry point for Distributors building custom binaries.
// It handles configuration loading, structured logging with redaction,
// admin server (health, version, pprof), TLS/mTLS, and graceful shutdown.
//
// The plugin parameter implements [sdk.Plugin] to provide credential
// injection logic. Pass nil to run without credential injection.
//
// Run calls [slog.SetDefault] to install a structured logger with
// header redaction. Callers sharing the process should be aware that
// this replaces the global logger.
//
// Run returns nil on clean shutdown (context cancelled) and a non-nil
// error for configuration or startup failures.
func Run(ctx context.Context, plugin sdk.Plugin, opts ...Option) error {
	rc := &runConfig{
		version:   "dev",
		logOutput: os.Stdout,
	}
	for _, opt := range opts {
		opt(rc)
	}

	// Load configuration.
	cfg, err := config.Load(rc.configPath)
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	configureLogging(rc, cfg)

	return startProxy(ctx, plugin, rc, cfg)
}

// configureLogging sets up the global slog logger with defense-in-depth
// redaction per Design Spec Section 5.3 (Sensitive Data Redaction).
func configureLogging(rc *runConfig, cfg *config.Config) {
	logLevel := parseLogLevel(cfg.Observability.LogLevel)
	slog.SetDefault(observability.NewLogger(
		rc.logOutput, logLevel,
		cfg.Observability.SensitiveHeaders,
		cfg.Observability.EnableBodyLogging,
	))

	if cfg.Observability.EnableBodyLogging {
		slog.Warn("body logging enabled — request/response bodies may appear in debug logs",
			"env_var", "CHAPERONE_OBSERVABILITY_ENABLE_BODY_LOGGING",
		)
	}
}

// startProxy wires up the admin and proxy servers, starts them, and blocks
// until ctx is cancelled or a fatal error occurs.
func startProxy(ctx context.Context, plugin sdk.Plugin, rc *runConfig, cfg *config.Config) error {
	logStartup(rc, cfg)

	// Initialize tracing (if enabled via OTEL_SDK_DISABLED != "true").
	tracingEnabled := telemetry.IsTracingEnabled()
	var shutdownTracing func(context.Context) error
	if tracingEnabled {
		var tracingErr error
		shutdownTracing, tracingErr = telemetry.InitTracing(context.Background(), telemetry.TracingConfig{ //nolint:contextcheck // tracing init is process-scoped, not request-scoped
			ServiceName:    "chaperone",
			ServiceVersion: rc.version,
			Enabled:        true,
		})
		if tracingErr != nil {
			return fmt.Errorf("initializing tracing: %w", tracingErr)
		}
	}

	// Start admin server (health, version, pprof, metrics).
	adminSrv := telemetry.NewAdminServer(cfg.Server.AdminAddr, rc.version)
	telemetry.RegisterPprofHandlers(adminSrv.Mux(), cfg.Observability.EnableProfiling)

	if startErr := adminSrv.Start(); startErr != nil { //nolint:contextcheck // AdminServer.Start binds a listener, no context needed
		return fmt.Errorf("starting admin server: %w", startErr)
	}

	// Ensure the admin server is cleaned up on any error path.
	// On the happy path awaitShutdown takes ownership; the flag
	// prevents a redundant (albeit safe) double-shutdown.
	var shutdownHandled bool
	defer func() {
		if !shutdownHandled {
			shutdownAdminServer(adminSrv) //nolint:contextcheck // fresh context needed; caller's ctx is unrelated to cleanup
		}
	}()

	srv, err := newProxyServer(plugin, rc, cfg, tracingEnabled)
	if err != nil {
		return err
	}

	// Start shutdown listener: when ctx is cancelled, drain servers gracefully.
	// A fresh Background context is used intentionally — the parent ctx is
	// already cancelled at this point, so we need a new deadline for draining.
	go awaitShutdown(ctx, srv, adminSrv, shutdownTracing, *cfg.Server.ShutdownTimeout)

	// Start blocks until the server stops.
	if startErr := srv.Start(); startErr != nil { //nolint:contextcheck // proxy.Server.Start blocks on Serve, no context parameter
		return fmt.Errorf("server error: %w", startErr)
	}

	shutdownHandled = true // awaitShutdown owns cleanup from here
	slog.Info("server stopped")
	return nil
}

// newProxyServer creates the proxy server from the loaded configuration.
func newProxyServer(plugin sdk.Plugin, rc *runConfig, cfg *config.Config, tracingEnabled bool) (*proxy.Server, error) {
	tlsEnabled := *cfg.Server.TLS.Enabled
	srv, err := proxy.NewServer(proxy.Config{
		Addr:             cfg.Server.Addr,
		Plugin:           plugin,
		Version:          rc.version,
		HeaderPrefix:     cfg.Upstream.HeaderPrefix,
		TraceHeader:      cfg.Upstream.TraceHeader,
		AllowList:        cfg.Upstream.AllowList,
		ConnectTimeout:   *cfg.Upstream.Timeouts.Connect,
		KeepAliveTimeout: *cfg.Upstream.Timeouts.KeepAlive,
		ShutdownTimeout:  *cfg.Server.ShutdownTimeout,
		PluginTimeout:    *cfg.Upstream.Timeouts.Plugin,
		SensitiveHeaders: cfg.Observability.SensitiveHeaders,
		TLS: &proxy.TLSConfig{
			Enabled:  tlsEnabled,
			CertFile: cfg.Server.TLS.CertFile,
			KeyFile:  cfg.Server.TLS.KeyFile,
			CAFile:   cfg.Server.TLS.CAFile,
		},
		ReadTimeout:    *cfg.Upstream.Timeouts.Read,
		WriteTimeout:   *cfg.Upstream.Timeouts.Write,
		IdleTimeout:    *cfg.Upstream.Timeouts.Idle,
		TracingEnabled: tracingEnabled,
	})
	if err != nil {
		return nil, fmt.Errorf("creating proxy server: %w", err)
	}
	return srv, nil
}

// awaitShutdown blocks until ctx is cancelled, then initiates a graceful
// shutdown of the traffic server, tracer provider, and admin server within
// the given timeout.
//
// Shutdown order: traffic server -> tracer flush -> admin server
// The traffic server is drained first (stops accepting new proxy requests),
// then traces are flushed, then the admin server (health checks remain
// available during traffic drain).
//
// A single timeout context is shared across all three shutdown phases.
// Under heavy load, earlier phases may consume most of the budget, leaving
// less time for tracer flush. This is acceptable because losing a few
// trailing traces is preferable to delaying shutdown.
//
// A fresh context (context.Background) is used intentionally because the
// parent ctx is already cancelled when this code runs.
//
//nolint:contextcheck // all contexts here are intentionally non-inherited from the cancelled parent
func awaitShutdown(ctx context.Context, srv *proxy.Server, adminSrv *telemetry.AdminServer, shutdownTracing func(context.Context) error, timeout time.Duration) {
	<-ctx.Done()
	slog.Info("shutdown signal received, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Drain traffic server first (stops accepting new proxy requests).
	srv.Shutdown(shutdownCtx)

	// Flush remaining traces before stopping admin.
	if shutdownTracing != nil {
		if err := shutdownTracing(shutdownCtx); err != nil {
			slog.Error("tracer provider shutdown error", "error", err)
		}
	}

	// Then drain admin server (health checks were available during traffic drain).
	if shutdownErr := adminSrv.Shutdown(shutdownCtx); shutdownErr != nil {
		slog.Error("admin server shutdown error", "error", shutdownErr)
	}
}

// logStartup emits the structured startup log line. Build metadata
// (commit, build_date) is included only when set via [WithBuildInfo].
func logStartup(rc *runConfig, cfg *config.Config) {
	attrs := []any{
		"version", rc.version,
		"config_addr", cfg.Server.Addr,
		"config_admin_addr", cfg.Server.AdminAddr,
		"log_level", cfg.Observability.LogLevel,
	}
	if rc.commit != "" {
		attrs = append(attrs, "commit", rc.commit)
	}
	if rc.buildDate != "" {
		attrs = append(attrs, "build_date", rc.buildDate)
	}
	slog.Info("starting chaperone", attrs...)
}

// shutdownAdminServer performs a best-effort shutdown of the admin server,
// used for cleanup when startProxy encounters an error after the admin
// server has already been started.
//
//nolint:contextcheck // intentionally uses context.Background; this is error-path cleanup, not request-scoped
func shutdownAdminServer(adminSrv *telemetry.AdminServer) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := adminSrv.Shutdown(ctx); err != nil {
		slog.Error("admin server cleanup failed", "error", err)
	}
}

// parseLogLevel converts a string log level to [slog.Level].
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
