// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for the Chaperone egress proxy.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/internal/telemetry"
	"github.com/cloudblue/chaperone/plugins/reference"
	"github.com/cloudblue/chaperone/sdk"
)

// Version information (set via ldflags during build)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

//nolint:funlen // CLI entry points are acceptable to be longer
func main() {
	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "enroll":
			enrollCmd(os.Args[2:])
			return
		case "help", "-h", "--help":
			if len(os.Args) > 2 && os.Args[2] == "enroll" {
				enrollCmd([]string{"-h"})
				return
			}
			// Fall through to normal flag parsing
		}
	}

	// Parse command line flags for the main server
	configPath := flag.String("config", "", "Path to config file (default: ./config.yaml or CHAPERONE_CONFIG env)")
	credFile := flag.String("credentials", "", "Path to credentials JSON file (optional)")
	showVersion := flag.Bool("version", false, "Show version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Chaperone - Secure Egress Proxy

Usage: chaperone [command] [options]

Commands:
  enroll      Generate CSR for production CA enrollment
  (default)   Start the proxy server

Server Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Configuration:
  Chaperone loads configuration from a YAML file with environment variable
  overrides. See configs/config.example.yaml for all options.

  Config file path resolution (in order):
    1. -config flag
    2. CHAPERONE_CONFIG environment variable
    3. ./config.yaml (default)

  Environment variables use pattern: CHAPERONE_<SECTION>_<KEY>
  Example: CHAPERONE_SERVER_ADDR=":9443"

Admin Server:
  Admin endpoints are served on server.admin_addr (default: 127.0.0.1:9090)
  Endpoints: /_ops/health, /debug/pprof/* (dev builds with profiling enabled)

  To enable profiling (dev builds only):
    - Set observability.enable_profiling: true in config
    - Or set CHAPERONE_OBSERVABILITY_ENABLE_PROFILING=true

Examples:
  chaperone                              # Start with default config
  chaperone -config /etc/chaperone.yaml  # Custom config path
  chaperone enroll --domains foo.com     # Generate production CSR
  chaperone -version                     # Show version
`)
	}

	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("Chaperone Egress Proxy\n")
		fmt.Printf("Version: %s\nCommit: %s\nBuilt: %s\n", Version, GitCommit, BuildDate)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Configure logging with defense-in-depth redaction.
	configureLogging(cfg)

	slog.Info("starting chaperone",
		"version", Version,
		"commit", GitCommit,
		"build_date", BuildDate,
		"config_addr", cfg.Server.Addr,
		"config_admin_addr", cfg.Server.AdminAddr,
		"log_level", cfg.Observability.LogLevel,
	)

	// Start admin server (health, pprof, future metrics)
	adminSrv := telemetry.NewAdminServer(cfg.Server.AdminAddr)

	// Register pprof handlers (dev builds only, when enabled via config)
	telemetry.RegisterPprofHandlers(adminSrv.Mux(), cfg.Observability.EnableProfiling)

	if startErr := adminSrv.Start(); startErr != nil {
		slog.Error("failed to start admin server", "error", startErr)
		os.Exit(1)
	}

	// Configure plugin (optional)
	var plugin sdk.Plugin
	if *credFile != "" {
		plugin = reference.New(*credFile)
		slog.Info("loaded reference plugin", "credentials_file", *credFile)
	} else {
		slog.Warn("no credentials file specified, running without credential injection")
	}

	// Create and start server using config values.
	tlsEnabled := *cfg.Server.TLS.Enabled
	srv, err := proxy.NewServer(proxy.Config{
		Addr:             cfg.Server.Addr,
		Plugin:           plugin,
		Version:          Version,
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
		ReadTimeout:  *cfg.Upstream.Timeouts.Read,
		WriteTimeout: *cfg.Upstream.Timeouts.Write,
		IdleTimeout:  *cfg.Upstream.Timeouts.Idle,
	})
	if err != nil {
		slog.Error("invalid server configuration", "error", err)
		os.Exit(1)
	}

	go awaitShutdown(srv, adminSrv, *cfg.Server.ShutdownTimeout)

	if err := srv.Start(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// awaitShutdown blocks until SIGTERM or SIGINT is received, then initiates
// a graceful shutdown of both the traffic and admin servers within the given
// timeout. The traffic server is drained first (stops accepting new proxy
// requests), then the admin server (health checks remain available during drain).
func awaitShutdown(srv *proxy.Server, adminSrv *telemetry.AdminServer, timeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	sig := <-quit
	slog.Info("received signal, initiating graceful shutdown",
		"signal", sig.String(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Drain traffic server first (stops accepting new proxy requests).
	srv.Shutdown(ctx)

	// Then drain admin server (health checks were available during traffic drain).
	if err := adminSrv.Shutdown(ctx); err != nil {
		slog.Error("admin server shutdown error", "error", err)
	}
}

// parseLogLevel converts a string log level to slog.Level.
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

// configureLogging sets up the global slog logger with defense-in-depth
// redaction per Design Spec Section 5.3 (Sensitive Data Redaction).
func configureLogging(cfg *config.Config) {
	logLevel := parseLogLevel(cfg.Observability.LogLevel)
	slog.SetDefault(observability.NewLogger(
		os.Stdout, logLevel,
		cfg.Observability.SensitiveHeaders,
		cfg.Observability.EnableBodyLogging,
	))

	// Security: Emit startup warning when body logging is enabled.
	// Per Design Spec Section 5.3 (Body Safety): body logging requires explicit
	// env var AND must emit a startup warning.
	if cfg.Observability.EnableBodyLogging {
		slog.Warn("body logging enabled — request/response bodies may appear in debug logs",
			"env_var", "CHAPERONE_OBSERVABILITY_ENABLE_BODY_LOGGING",
		)
	}
}
