// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for the Chaperone egress proxy.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloudblue/chaperone/internal/config"
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
  Admin endpoints are served on server.admin_addr (default: :9090)
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

	// Configure logging based on config
	logLevel := parseLogLevel(cfg.Observability.LogLevel)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

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

	if err := adminSrv.Start(); err != nil {
		slog.Error("failed to start admin server", "error", err)
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

	// Create and start server using config values
	tlsEnabled := *cfg.Server.TLS.Enabled
	srv := proxy.NewServer(proxy.Config{
		Addr:         cfg.Server.Addr,
		Plugin:       plugin,
		Version:      Version,
		HeaderPrefix: cfg.Upstream.HeaderPrefix,
		AllowList:    cfg.Upstream.AllowList,
		TLS: &proxy.TLSConfig{
			Enabled:  tlsEnabled,
			CertFile: cfg.Server.TLS.CertFile,
			KeyFile:  cfg.Server.TLS.KeyFile,
			CAFile:   cfg.Server.TLS.CAFile,
		},
		ReadTimeout:  cfg.Upstream.Timeouts.Read,
		WriteTimeout: cfg.Upstream.Timeouts.Write,
		IdleTimeout:  cfg.Upstream.Timeouts.Idle,
	})

	if tlsEnabled {
		slog.Info("server listening with mTLS (Mode A)", "addr", cfg.Server.Addr)
	} else {
		slog.Info("server listening without TLS (Mode B)", "addr", cfg.Server.Addr)
	}
	if err := srv.Start(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
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
