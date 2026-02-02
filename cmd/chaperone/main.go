// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for the Chaperone egress proxy.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloudblue/chaperone/internal/proxy"
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
	addr := flag.String("addr", ":8443", "Address to listen on")
	credFile := flag.String("credentials", "", "Path to credentials JSON file (optional)")
	tlsEnabled := flag.Bool("tls", true, "Enable mTLS (Mode A)")
	certFile := flag.String("cert", "certs/server.crt", "Path to server certificate")
	keyFile := flag.String("key", "certs/server.key", "Path to server private key")
	caFile := flag.String("ca", "certs/ca.crt", "Path to CA certificate for client verification")
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
Examples:
  chaperone                           # Start server with defaults
  chaperone -addr :9443               # Start on different port
  chaperone enroll --domains foo.com  # Generate production CSR
  chaperone -version                  # Show version
`)
	}

	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("Chaperone Egress Proxy\n")
		fmt.Printf("Version: %s\nCommit: %s\nBuilt: %s\n", Version, GitCommit, BuildDate)
		os.Exit(0)
	}

	// Configure logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("starting chaperone",
		"version", Version,
		"commit", GitCommit,
		"build_date", BuildDate,
	)

	// Configure plugin (optional)
	var plugin sdk.Plugin
	if *credFile != "" {
		plugin = reference.New(*credFile)
		slog.Info("loaded reference plugin", "credentials_file", *credFile)
	} else {
		slog.Warn("no credentials file specified, running without credential injection")
	}

	// Create and start server
	srv := proxy.NewServer(proxy.Config{
		Addr:    *addr,
		Plugin:  plugin,
		Version: Version,
		TLS: &proxy.TLSConfig{
			Enabled:  *tlsEnabled,
			CertFile: *certFile,
			KeyFile:  *keyFile,
			CAFile:   *caFile,
		},
	})

	if *tlsEnabled {
		slog.Info("server listening with mTLS (Mode A)", "addr", *addr)
	} else {
		slog.Info("server listening in HTTP mode (Mode B)", "addr", *addr)
	}
	if err := srv.Start(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
