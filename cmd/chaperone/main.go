// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for the Chaperone egress proxy.
//
// This CLI wraps [chaperone.Run] with additional conveniences:
//   - Subcommands (enroll)
//   - CLI flags (-config, -credentials, -version)
//   - Reference plugin integration
//
// Distributors building their own binary should use [chaperone.Run] directly.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudblue/chaperone"
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

	// Configure plugin (optional).
	// Note: these slog calls use the default handler (no redaction) because
	// Run() hasn't configured the redacting logger yet. This is safe because
	// neither message includes sensitive data.
	var plugin sdk.Plugin
	if *credFile != "" {
		plugin = reference.New(*credFile)
		slog.Info("loaded reference plugin", "credentials_file", *credFile)
	} else {
		slog.Warn("no credentials file specified, running without credential injection")
	}

	if err := run(*configPath, plugin); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

// run creates a signal-aware context and delegates to the public API.
func run(configPath string, plugin sdk.Plugin) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	return chaperone.Run(ctx, plugin,
		chaperone.WithConfigPath(configPath),
		chaperone.WithVersion(Version),
		chaperone.WithBuildInfo(GitCommit, BuildDate),
	)
}
